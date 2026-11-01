package backup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"github.com/google/uuid"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Entry struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type Manifest struct {
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	Files     []Entry   `json:"files"`
	Directory string    `json:"-"`
}

func Create(ctx context.Context, dataDir, backupDir string) (Manifest, error) {
	if e := os.MkdirAll(backupDir, 0700); e != nil {
		return Manifest{}, e
	}
	restoreLease, e := storage.AcquireFileLock(dataDir, "restore.lock", false, true)
	if e != nil {
		return Manifest{}, e
	}
	defer restoreLease.Close()
	if _, e = os.Stat(filepath.Join(dataDir, "restore-in-progress")); e == nil {
		return Manifest{}, errors.New("incomplete restore")
	}
	guard, e := storage.AcquireFileLock(backupDir, ".backup.lock", true, true)
	if e != nil {
		return Manifest{}, e
	}
	defer guard.Close()
	mediaLease, e := storage.AcquireFileLock(dataDir, "media.lock", false, false)
	if e != nil {
		return Manifest{}, e
	}
	defer mediaLease.Close()
	staging, e := os.MkdirTemp(backupDir, ".incomplete-")
	if e != nil {
		return Manifest{}, e
	}
	defer os.RemoveAll(staging)
	db, e := storage.OpenExisting(ctx, dataDir, false)
	if e != nil {
		return Manifest{}, e
	}
	defer db.Close()
	if _, e = db.ExecContext(ctx, "VACUUM INTO ?", filepath.Join(staging, "knowledgebase.db")); e != nil {
		return Manifest{}, e
	}
	snapshot, e := storage.OpenExisting(ctx, staging, true)
	if e != nil {
		return Manifest{}, e
	}
	ids := []string{}
	rows, e := snapshot.QueryContext(ctx, "SELECT id FROM media ORDER BY id")
	if e != nil {
		snapshot.Close()
		return Manifest{}, e
	}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			break
		}
		ids = append(ids, id)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e == nil {
		e = integrity(ctx, snapshot)
	}
	snapshot.Close()
	if e != nil {
		return Manifest{}, e
	}
	m := Manifest{Version: 1, CreatedAt: time.Now().UTC(), Files: []Entry{}}
	entry, e := digestFile(ctx, filepath.Join(staging, "knowledgebase.db"))
	if e != nil {
		return Manifest{}, e
	}
	entry.Name = "knowledgebase.db"
	m.Files = append(m.Files, entry)
	names := []string{}
	if _, e = os.Stat(filepath.Join(dataDir, "csrf.key")); e == nil {
		names = append(names, "csrf.key")
	} else if !os.IsNotExist(e) {
		return Manifest{}, e
	}
	for _, id := range ids {
		if _, e = uuid.Parse(id); e != nil {
			return Manifest{}, fmt.Errorf("invalid media ID")
		}
		names = append(names, "media/"+id)
	}
	for _, name := range names {
		entry, e = copyFile(ctx, filepath.Join(dataDir, filepath.FromSlash(name)), filepath.Join(staging, filepath.FromSlash(name)))
		if e != nil {
			return Manifest{}, e
		}
		entry.Name = name
		m.Files = append(m.Files, entry)
	}
	if _, e = os.Stat(filepath.Join(staging, "media")); e == nil {
		if e = syncDir(filepath.Join(staging, "media")); e != nil {
			return Manifest{}, e
		}
	} else if !os.IsNotExist(e) {
		return Manifest{}, e
	}
	b, e := json.MarshalIndent(m, "", "  ")
	if e != nil {
		return Manifest{}, e
	}
	if e = writeSync(filepath.Join(staging, "manifest.json"), b); e != nil {
		return Manifest{}, e
	}
	if e = syncDir(staging); e != nil {
		return Manifest{}, e
	}
	final := filepath.Join(backupDir, "kb-"+m.CreatedAt.Format("20060102T150405.000000000Z")+"-"+uuid.NewString())
	if e = os.Rename(staging, final); e != nil {
		return Manifest{}, e
	}
	if e = syncDir(backupDir); e != nil {
		return Manifest{}, e
	}
	m.Directory = final
	if e = rotateLocked(ctx, backupDir, 14); e != nil {
		return m, e
	}
	return m, nil
}
func Rotate(ctx context.Context, dir string, keep int) error {
	guard, e := storage.AcquireFileLock(dir, ".backup.lock", true, true)
	if e != nil {
		return e
	}
	defer guard.Close()
	return rotateLocked(ctx, dir, keep)
}
func rotateLocked(ctx context.Context, dir string, keep int) error {
	if keep < 1 {
		return errors.New("retention must be positive")
	}
	entries, e := os.ReadDir(dir)
	if e != nil {
		return e
	}
	names := []string{}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "kb-") {
			if _, e = readManifest(filepath.Join(dir, entry.Name())); e != nil {
				continue
			}
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for len(names) > keep {
		if e = ctx.Err(); e != nil {
			return e
		}
		if e = os.RemoveAll(filepath.Join(dir, names[0])); e != nil {
			return e
		}
		names = names[1:]
	}
	return syncDir(dir)
}
func integrity(ctx context.Context, db *sql.DB) error {
	rows, e := db.QueryContext(ctx, "PRAGMA integrity_check")
	if e != nil {
		return e
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var result string
		if e = rows.Scan(&result); e != nil {
			return e
		}
		if result != "ok" {
			return errors.New("SQLite integrity check failed")
		}
		n++
	}
	if e = rows.Err(); e != nil {
		return e
	}
	if n != 1 {
		return errors.New("missing integrity result")
	}
	var violations int
	rs, e := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if e != nil {
		return e
	}
	defer rs.Close()
	if rs.Next() {
		violations++
	}
	if violations != 0 {
		return errors.New("SQLite foreign key check failed")
	}
	return rs.Err()
}
func readManifest(dir string) (Manifest, error) {
	var m Manifest
	f, e := os.Open(filepath.Join(dir, "manifest.json"))
	if e != nil {
		return m, e
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 16<<20))
	d.DisallowUnknownFields()
	if e = d.Decode(&m); e != nil {
		return m, e
	}
	if d.Decode(new(any)) != io.EOF || m.Version != 1 || m.CreatedAt.IsZero() {
		return m, errors.New("invalid backup manifest")
	}
	seen := map[string]bool{}
	for _, entry := range m.Files {
		if seen[entry.Name] || entry.Size < 0 || len(entry.SHA256) != 64 {
			return m, errors.New("invalid backup entry")
		}
		if _, e = hex.DecodeString(entry.SHA256); e != nil {
			return m, e
		}
		seen[entry.Name] = true
		if entry.Name == "knowledgebase.db" || entry.Name == "csrf.key" {
			continue
		}
		if !strings.HasPrefix(entry.Name, "media/") {
			return m, errors.New("unsafe backup path")
		}
		if _, e = uuid.Parse(strings.TrimPrefix(entry.Name, "media/")); e != nil {
			return m, errors.New("unsafe backup media path")
		}
	}
	if !seen["knowledgebase.db"] {
		return m, errors.New("missing database")
	}
	m.Directory = dir
	return m, nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (c contextReader) Read(p []byte) (int, error) {
	if e := c.ctx.Err(); e != nil {
		return 0, e
	}
	return c.r.Read(p)
}
func regular(path string) (*os.File, error) {
	info, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("backup source must be a regular file")
	}
	return os.Open(path)
}
func digestFile(ctx context.Context, path string) (Entry, error) {
	f, e := regular(path)
	if e != nil {
		return Entry{}, e
	}
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(h, contextReader{ctx, f})
	return Entry{Size: n, SHA256: hex.EncodeToString(h.Sum(nil))}, e
}
func copyFile(ctx context.Context, source, target string) (Entry, error) {
	f, e := regular(source)
	if e != nil {
		return Entry{}, e
	}
	defer f.Close()
	if e = os.MkdirAll(filepath.Dir(target), 0700); e != nil {
		return Entry{}, e
	}
	out, e := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return Entry{}, e
	}
	defer out.Close()
	h := sha256.New()
	n, e := io.Copy(io.MultiWriter(out, h), contextReader{ctx, f})
	if e != nil {
		return Entry{}, e
	}
	if e = out.Sync(); e != nil {
		return Entry{}, e
	}
	return Entry{Size: n, SHA256: hex.EncodeToString(h.Sum(nil))}, out.Close()
}
func writeSync(path string, b []byte) error {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if _, e = f.Write(b); e != nil {
		return e
	}
	return f.Sync()
}
func syncDir(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
