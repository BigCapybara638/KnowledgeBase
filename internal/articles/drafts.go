package articles

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/BigCapybara638/KnowledgeBase/internal/content"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"strings"
	"time"
)

type SaveDraftInput struct {
	DraftID, ActorID, CategoryID domain.ID
	ExpectedVersion              domain.Version
	Title                        string
	Document                     json.RawMessage
}

const emptyDocument = `{"type":"doc","content":[{"type":"paragraph"}]}`
const draftSelect = "SELECT id,article_id,owner_id,COALESCE(category_id,''),title,document,version,base_article_version,updated_at FROM drafts "

func scanDraft(row scanner) (domain.Draft, error) {
	var d domain.Draft
	var doc string
	e := row.Scan(&d.ID, &d.ArticleID, &d.OwnerID, &d.CategoryID, &d.Title, &doc, &d.Version, &d.BaseArticleVersion, &d.UpdatedAt)
	d.Document = []byte(doc)
	if errors.Is(e, sql.ErrNoRows) {
		e = domain.ErrNotFound
	}
	return d, e
}
func (s *Service) GetDraft(ctx context.Context, actor, id domain.ID) (domain.Draft, error) {
	if _, e := s.store.Editor(ctx, actor); e != nil {
		return domain.Draft{}, e
	}
	return scanDraft(s.store.Read.QueryRowContext(ctx, draftSelect+"WHERE id=? AND owner_id=?", id, actor))
}
func (s *Service) Drafts(ctx context.Context, actor domain.ID) ([]domain.Draft, error) {
	if _, e := s.store.Editor(ctx, actor); e != nil {
		return nil, e
	}
	rows, e := s.store.Read.QueryContext(ctx, draftSelect+"WHERE owner_id=? ORDER BY updated_at DESC,id", actor)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.Draft{}
	for rows.Next() {
		d, e := scanDraft(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (s *Service) CreateDraft(ctx context.Context, actor domain.ID, articleID *domain.ID) (domain.Draft, error) {
	if _, e := s.store.Editor(ctx, actor); e != nil {
		return domain.Draft{}, e
	}
	d := domain.Draft{ID: domain.NewID(), ArticleID: domain.NewID(), OwnerID: actor, Version: 1, Document: []byte(emptyDocument), UpdatedAt: time.Now().Unix()}
	e := s.store.WithWrite(ctx, func(tx *sql.Tx) error {
		if e := storage.AssertActor(ctx, tx, actor, false); e != nil {
			return e
		}
		if articleID == nil {
			if _, e := tx.ExecContext(ctx, "INSERT INTO articles(id,author_id) VALUES(?,?)", d.ArticleID, actor); e != nil {
				return e
			}
		} else {
			d.ArticleID = *articleID
			var doc string
			e := tx.QueryRowContext(ctx, "SELECT COALESCE(category_id,''),title,document,version FROM articles WHERE id=? AND published_at IS NOT NULL AND archived=0", d.ArticleID).Scan(&d.CategoryID, &d.Title, &doc, &d.BaseArticleVersion)
			if errors.Is(e, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			if e != nil {
				return e
			}
			d.Document = []byte(doc)
		}
		_, e := tx.ExecContext(ctx, "INSERT INTO drafts(id,article_id,owner_id,category_id,title,document,base_article_version,updated_at) VALUES(?,?,?,?,?,?,?,?)", d.ID, d.ArticleID, actor, nullableID(d.CategoryID), d.Title, string(d.Document), d.BaseArticleVersion, d.UpdatedAt)
		if e != nil {
			return e
		}
		if articleID != nil {
			_, e = tx.ExecContext(ctx, "INSERT INTO draft_media(draft_id,media_id) SELECT ?,media_id FROM article_media WHERE article_id=?", d.ID, d.ArticleID)
		}
		return e
	})
	return d, e
}
func nullableID(id domain.ID) any {
	if id == "" {
		return nil
	}
	return string(id)
}
func (s *Service) SaveDraft(ctx context.Context, in SaveDraftInput) (domain.Draft, error) {
	if _, e := s.store.Editor(ctx, in.ActorID); e != nil {
		return domain.Draft{}, e
	}
	if len(in.Title) > 512 {
		return domain.Draft{}, domain.ErrInvalid
	}
	rendered, e := content.ValidateAndRender(in.Document)
	if e != nil {
		return domain.Draft{}, e
	}
	var saved domain.Draft
	e = s.store.WithWrite(ctx, func(tx *sql.Tx) error {
		if e := storage.AssertActor(ctx, tx, in.ActorID, false); e != nil {
			return e
		}
		if in.CategoryID != "" {
			var n int
			if e := tx.QueryRowContext(ctx, "SELECT count(*) FROM categories WHERE id=?", in.CategoryID).Scan(&n); e != nil {
				return e
			}
			if n != 1 {
				return domain.ErrInvalid
			}
		}
		d, e := scanDraft(tx.QueryRowContext(ctx, draftSelect+"WHERE id=? AND owner_id=?", in.DraftID, in.ActorID))
		if e != nil {
			return e
		}
		if d.Version != in.ExpectedVersion {
			return domain.ErrConflict
		}
		if e = syncDraftMedia(ctx, tx, in.ActorID, in.DraftID, rendered.MediaIDs); e != nil {
			return e
		}
		r, e := tx.ExecContext(ctx, "UPDATE drafts SET title=?,category_id=?,document=?,version=version+1,updated_at=? WHERE id=? AND owner_id=? AND version=?", strings.TrimSpace(in.Title), nullableID(in.CategoryID), string(in.Document), time.Now().Unix(), in.DraftID, in.ActorID, in.ExpectedVersion)
		if e != nil {
			return e
		}
		n, _ := r.RowsAffected()
		if n != 1 {
			return domain.ErrConflict
		}
		saved, e = scanDraft(tx.QueryRowContext(ctx, draftSelect+"WHERE id=? AND owner_id=?", in.DraftID, in.ActorID))
		return e
	})
	if e != nil {
		return domain.Draft{}, e
	}
	return saved, nil
}
func syncDraftMedia(ctx context.Context, tx *sql.Tx, actor, draft domain.ID, ids []domain.ID) error {
	for _, id := range ids {
		var n int
		e := tx.QueryRowContext(ctx, `SELECT count(*) FROM media m WHERE m.id=? AND (m.owner_id=? OR EXISTS(SELECT 1 FROM draft_media dm JOIN drafts d ON d.id=dm.draft_id WHERE dm.media_id=m.id AND d.owner_id=?) OR EXISTS(SELECT 1 FROM article_media am JOIN articles a ON a.id=am.article_id WHERE am.media_id=m.id AND a.published_at IS NOT NULL AND a.archived=0))`, id, actor, actor).Scan(&n)
		if e != nil {
			return e
		}
		if n != 1 {
			return domain.ErrForbidden
		}
	}
	if _, e := tx.ExecContext(ctx, "DELETE FROM draft_media WHERE draft_id=?", draft); e != nil {
		return e
	}
	for _, id := range ids {
		if _, e := tx.ExecContext(ctx, "INSERT OR IGNORE INTO draft_media(draft_id,media_id) VALUES(?,?)", draft, id); e != nil {
			return e
		}
	}
	return nil
}
