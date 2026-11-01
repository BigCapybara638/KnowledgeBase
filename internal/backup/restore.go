package backup

import (
	"context"
	"errors"
	"fmt"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"os"
	"path/filepath"
	"strings"
)

func Restore(ctx context.Context, bundleDir, targetDir string) error {
	bundleDir, e := filepath.Abs(bundleDir)
	if e != nil {
		return e
	}
	targetDir, e = filepath.Abs(targetDir)
	if e != nil {
		return e
	}
	if targetDir == bundleDir || strings.HasPrefix(bundleDir, targetDir+string(os.PathSeparator)) {
		return errors.New("restore target must be separate from backup bundle")
	}
	sourceGuard, e := storage.AcquireFileLock(filepath.Dir(bundleDir), ".backup.lock", false, true)
	if e != nil {
		return e
	}
	defer sourceGuard.Close()
	m, e := readManifest(bundleDir)
	if e != nil {
		return e
	}
	if info, e := os.Lstat(targetDir); e == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return errors.New("restore target must be a real directory")
	}
	instance, e := storage.AcquireInstanceLock(targetDir)
	if e != nil {
		return e
	}
	defer instance.Close()
	lease, e := storage.AcquireFileLock(targetDir, "restore.lock", true, true)
	if e != nil {
		return e
	}
	defer lease.Close()
	entries, e := os.ReadDir(targetDir)
	if e != nil {
		return e
	}
	for _, entry := range entries {
		if entry.Name() != "instance.lock" && entry.Name() != "restore.lock" {
			return errors.New("restore target is not empty; preserve it and choose an empty directory")
		}
	}
	staging, e := os.MkdirTemp(filepath.Dir(targetDir), ".kb-restore-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(staging)
	for _, entry := range m.Files {
		if strings.HasPrefix(entry.Name, "media/") {
			info, e := os.Lstat(filepath.Join(bundleDir, "media"))
			if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return errors.New("unsafe media directory")
			}
		}
		copied, e := copyFile(ctx, filepath.Join(bundleDir, filepath.FromSlash(entry.Name)), filepath.Join(staging, filepath.FromSlash(entry.Name)))
		if e != nil {
			return e
		}
		if copied.Size != entry.Size || copied.SHA256 != entry.SHA256 {
			return fmt.Errorf("checksum mismatch: %s", entry.Name)
		}
	}
	db, e := storage.OpenExisting(ctx, staging, false)
	if e != nil {
		return e
	}
	if e = integrity(ctx, db); e == nil {
		_, e = db.ExecContext(ctx, "DELETE FROM sessions")
	}
	closeErr := db.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	marker := filepath.Join(targetDir, "restore-in-progress")
	if e = writeSync(marker, []byte("Incomplete restore. Preserve this directory and restore again into an empty one.\n")); e != nil {
		return e
	}
	if e = syncDir(targetDir); e != nil {
		return e
	}
	for _, name := range []string{"knowledgebase.db", "csrf.key", "media"} {
		source := filepath.Join(staging, name)
		if _, e = os.Stat(source); os.IsNotExist(e) {
			continue
		} else if e != nil {
			return e
		}
		if e = os.Rename(source, filepath.Join(targetDir, name)); e != nil {
			return e
		}
	}
	if e = syncDir(targetDir); e != nil {
		return e
	}
	if e = os.Remove(marker); e != nil {
		return e
	}
	return syncDir(targetDir)
}
