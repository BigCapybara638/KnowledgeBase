package articles

import (
	"context"
	"database/sql"
	"errors"
	"github.com/BigCapybara638/KnowledgeBase/internal/content"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"strings"
	"time"
)

func (s *Service) Publish(ctx context.Context, actor, draftID domain.ID, expectedDraft, expectedArticle domain.Version) (domain.Article, error) {
	if _, e := s.store.Editor(ctx, actor); e != nil {
		return domain.Article{}, e
	}
	var articleID domain.ID
	var persisted domain.Article
	// The expensive JSON validation happens before taking the writer connection.
	d, e := s.GetDraft(ctx, actor, draftID)
	if errors.Is(e, domain.ErrNotFound) {
		return domain.Article{}, domain.ErrConflict
	}
	if e != nil {
		return domain.Article{}, e
	}
	rendered, e := content.ValidateAndRender(d.Document)
	if e != nil {
		return domain.Article{}, e
	}
	if strings.TrimSpace(d.Title) == "" || d.CategoryID == "" {
		return domain.Article{}, domain.ErrInvalid
	}
	e = s.store.WithWrite(ctx, func(tx *sql.Tx) error {
		if e := storage.AssertActor(ctx, tx, actor, false); e != nil {
			return e
		}
		current, e := scanDraft(tx.QueryRowContext(ctx, draftSelect+"WHERE id=? AND owner_id=?", draftID, actor))
		if errors.Is(e, domain.ErrNotFound) {
			return domain.ErrConflict
		}
		if e != nil {
			return e
		}
		if current.Version != expectedDraft || current.Version != d.Version || current.BaseArticleVersion != expectedArticle {
			return domain.ErrConflict
		}
		articleID = d.ArticleID
		now := time.Now().Unix()
		r, e := tx.ExecContext(ctx, "UPDATE articles SET title=?,category_id=?,document=?,plain_text=?,last_editor_id=?,version=version+1,published_at=COALESCE(published_at,?),updated_at=? WHERE id=? AND version=? AND archived=0", d.Title, d.CategoryID, string(d.Document), rendered.PlainText, actor, now, now, articleID, expectedArticle)
		if e != nil {
			return e
		}
		n, _ := r.RowsAffected()
		if n != 1 {
			return domain.ErrConflict
		}
		if e = syncDraftMedia(ctx, tx, actor, draftID, rendered.MediaIDs); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "DELETE FROM article_media WHERE article_id=?", articleID); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO article_media(article_id,media_id) SELECT ?,media_id FROM draft_media WHERE draft_id=?", articleID, draftID); e != nil {
			return e
		}
		if e = indexArticle(ctx, tx, articleID, d.Title, rendered.PlainText, false); e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, "DELETE FROM drafts WHERE id=? AND version=?", draftID, expectedDraft)
		if e != nil {
			return e
		}
		persisted, e = scanArticle(tx.QueryRowContext(ctx, publishedSelect+"WHERE a.id=?", articleID))
		return e
	})
	if e != nil {
		return domain.Article{}, e
	}
	return persisted, nil
}
func indexArticle(ctx context.Context, tx *sql.Tx, id domain.ID, title, body string, archived bool) error {
	if _, e := tx.ExecContext(ctx, "DELETE FROM articles_fts WHERE article_id=?", id); e != nil {
		return e
	}
	if archived {
		return nil
	}
	_, e := tx.ExecContext(ctx, "INSERT INTO articles_fts(article_id,title,body) VALUES(?,?,?)", id, title, body)
	return e
}
func (s *Service) SetArchived(ctx context.Context, admin, id domain.ID, expected domain.Version, archived bool) error {
	if e := s.store.Admin(ctx, admin); e != nil {
		return e
	}
	return s.store.WithWrite(ctx, func(tx *sql.Tx) error {
		if e := storage.AssertActor(ctx, tx, admin, true); e != nil {
			return e
		}
		r, e := tx.ExecContext(ctx, "UPDATE articles SET archived=?,version=version+1,updated_at=? WHERE id=? AND version=? AND published_at IS NOT NULL", archived, time.Now().Unix(), id, expected)
		if e != nil {
			return e
		}
		n, _ := r.RowsAffected()
		if n != 1 {
			return domain.ErrConflict
		}
		var title, body string
		if e = tx.QueryRowContext(ctx, "SELECT title,plain_text FROM articles WHERE id=?", id).Scan(&title, &body); e != nil {
			return e
		}
		return indexArticle(ctx, tx, id, title, body, archived)
	})
}
func (s *Service) Archived(ctx context.Context, admin domain.ID) ([]domain.Article, error) {
	if e := s.store.Admin(ctx, admin); e != nil {
		return nil, e
	}
	rows, e := s.store.Read.QueryContext(ctx, publishedSelect+"WHERE a.published_at IS NOT NULL AND a.archived=1 ORDER BY a.updated_at DESC,a.id")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.Article{}
	for rows.Next() {
		a, e := scanArticle(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
