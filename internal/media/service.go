package media

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"github.com/google/uuid"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrUploadLimit = errors.New("too many uploads")

type Service struct {
	store *storage.Store
	slots chan struct{}
}
type File struct {
	*os.File
	Media domain.Media
}

func New(s *storage.Store) *Service { return &Service{store: s, slots: make(chan struct{}, 2)} }
func (s *Service) Upload(ctx context.Context, actor, draft domain.ID, source io.Reader, filename string) (domain.Media, error) {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return domain.Media{}, ErrUploadLimit
	}
	if _, e := s.store.Editor(ctx, actor); e != nil {
		return domain.Media{}, e
	}
	var owns int
	if e := s.store.Read.QueryRowContext(ctx, "SELECT count(*) FROM drafts WHERE id=? AND owner_id=?", draft, actor).Scan(&owns); e != nil {
		return domain.Media{}, e
	}
	if owns != 1 {
		return domain.Media{}, domain.ErrNotFound
	}
	if filename == "" || filepath.Base(filename) != filename || strings.ContainsAny(filename, "\\\r\n") {
		return domain.Media{}, domain.ErrInvalid
	}
	b, e := io.ReadAll(io.LimitReader(source, (10<<20)+1))
	if e != nil {
		return domain.Media{}, e
	}
	if len(b) > 10<<20 {
		return domain.Media{}, domain.ErrInvalid
	}
	cfg := image.Config{}
	mime := http.DetectContentType(b)
	if mime != "video/mp4" && mime != "video/webm" && mime != "video/ogg" {
		var format string
		cfg, format, e = image.DecodeConfig(bytes.NewReader(b))
		if e != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 25000000/cfg.Height {
			return domain.Media{}, domain.ErrInvalid
		}
		mime = map[string]string{"jpeg": "image/jpeg", "png": "image/png", "webp": "image/webp"}[format]
		if mime == "" {
			return domain.Media{}, domain.ErrInvalid
		}
		if _, _, e = image.Decode(bytes.NewReader(b)); e != nil {
			return domain.Media{}, domain.ErrInvalid
		}
	}
	lock, e := storage.AcquireFileLock(s.store.DataDir, "media.lock", false, false)
	if e != nil {
		return domain.Media{}, e
	}
	defer lock.Close()
	dir := filepath.Join(s.store.DataDir, "media")
	if e = os.MkdirAll(dir, 0700); e != nil {
		return domain.Media{}, e
	}
	tmp, e := os.CreateTemp(dir, ".upload-")
	if e != nil {
		return domain.Media{}, e
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	defer tmp.Close()
	if _, e = tmp.Write(b); e != nil {
		return domain.Media{}, e
	}
	if e = tmp.Sync(); e != nil {
		return domain.Media{}, e
	}
	if e = tmp.Close(); e != nil {
		return domain.Media{}, e
	}
	m := domain.Media{ID: domain.NewID(), Type: mime, Size: int64(len(b)), Width: cfg.Width, Height: cfg.Height}
	final := filepath.Join(dir, string(m.ID))
	if e = os.Rename(tmpName, final); e != nil {
		return domain.Media{}, e
	}
	e = s.store.WithWrite(ctx, func(tx *sql.Tx) error {
		if e := storage.AssertActor(ctx, tx, actor, false); e != nil {
			return e
		}
		var n int
		if e := tx.QueryRowContext(ctx, "SELECT count(*) FROM drafts WHERE id=? AND owner_id=?", draft, actor).Scan(&n); e != nil {
			return e
		}
		if n != 1 {
			return domain.ErrNotFound
		}
		if _, e := tx.ExecContext(ctx, "INSERT INTO media(id,owner_id,mime,size,width,height,created_at) VALUES(?,?,?,?,?,?,?)", m.ID, actor, m.Type, m.Size, m.Width, m.Height, time.Now().Unix()); e != nil {
			return e
		}
		_, e := tx.ExecContext(ctx, "INSERT INTO draft_media(draft_id,media_id) VALUES(?,?)", draft, m.ID)
		return e
	})
	if e != nil {
		os.Remove(final)
		return domain.Media{}, e
	}
	return m, nil
}
func (s *Service) Open(ctx context.Context, actor, id domain.ID) (File, error) {
	if _, e := s.store.Editor(ctx, actor); e != nil {
		return File{}, e
	}
	if _, e := uuid.Parse(string(id)); e != nil {
		return File{}, domain.ErrNotFound
	}
	var m domain.Media
	e := s.store.Read.QueryRowContext(ctx, `SELECT id,mime,size,width,height FROM media m WHERE id=? AND (owner_id=? OR EXISTS(SELECT 1 FROM draft_media dm JOIN drafts d ON d.id=dm.draft_id WHERE dm.media_id=m.id AND d.owner_id=?) OR EXISTS(SELECT 1 FROM article_media am JOIN articles a ON a.id=am.article_id WHERE am.media_id=m.id AND a.published_at IS NOT NULL AND a.archived=0))`, id, actor, actor).Scan(&m.ID, &m.Type, &m.Size, &m.Width, &m.Height)
	if errors.Is(e, sql.ErrNoRows) {
		return File{}, domain.ErrNotFound
	}
	if e != nil {
		return File{}, e
	}
	f, e := os.Open(filepath.Join(s.store.DataDir, "media", string(id)))
	if os.IsNotExist(e) {
		return File{}, domain.ErrNotFound
	}
	return File{File: f, Media: m}, e
}
func (s *Service) CollectOrphans(ctx context.Context, olderThan time.Time) (int, error) {
	if bound := time.Now().Add(-24 * time.Hour); olderThan.After(bound) {
		olderThan = bound
	}
	lock, e := storage.AcquireFileLock(s.store.DataDir, "media.lock", true, false)
	if e != nil {
		return 0, e
	}
	defer lock.Close()
	rows, e := s.store.Read.QueryContext(ctx, "SELECT id FROM media WHERE created_at<? AND NOT EXISTS(SELECT 1 FROM draft_media WHERE media_id=media.id) AND NOT EXISTS(SELECT 1 FROM article_media WHERE media_id=media.id)", olderThan.Unix())
	if e != nil {
		return 0, e
	}
	ids := []domain.ID{}
	for rows.Next() {
		var id domain.ID
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return 0, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return 0, e
	}
	count := 0
	for _, id := range ids {
		e = s.store.WithWrite(ctx, func(tx *sql.Tx) error {
			var refs int
			if e := tx.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM draft_media WHERE media_id=?)+(SELECT count(*) FROM article_media WHERE media_id=?)", id, id).Scan(&refs); e != nil {
				return e
			}
			if refs != 0 {
				return nil
			}
			if _, e := uuid.Parse(string(id)); e != nil {
				return domain.ErrInvalid
			}
			e := os.Remove(filepath.Join(s.store.DataDir, "media", string(id)))
			if e != nil && !os.IsNotExist(e) {
				return e
			}
			if _, e = tx.ExecContext(ctx, "DELETE FROM media WHERE id=?", id); e == nil {
				count++
			}
			return e
		})
		if e != nil {
			return count, e
		}
	}
	dir := filepath.Join(s.store.DataDir, "media")
	entries, e := os.ReadDir(dir)
	if os.IsNotExist(e) {
		return count, nil
	}
	if e != nil {
		return count, e
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		info, e := entry.Info()
		if e != nil {
			return count, e
		}
		if !info.ModTime().Before(olderThan) {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".upload-") {
			if e = os.Remove(filepath.Join(dir, name)); e != nil {
				return count, e
			}
			continue
		}
		if _, e = uuid.Parse(name); e != nil {
			continue
		}
		var n int
		if e = s.store.Read.QueryRowContext(ctx, "SELECT count(*) FROM media WHERE id=?", name).Scan(&n); e != nil {
			return count, e
		}
		if n == 0 {
			if e = os.Remove(filepath.Join(dir, name)); e != nil {
				return count, e
			}
		}
	}
	return count, nil
}
