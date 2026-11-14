package articles

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/BigCapybara638/KnowledgeBase/internal/content"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
)

type CreateDraftInput struct {
	Title      string
	CategoryID domain.ID
	Document   json.RawMessage
}

// CreateFilledDraft atomically creates a new article placeholder and its full
// personal draft. Invalid content never leaves an empty article behind.
func (s *Service) CreateFilledDraft(ctx context.Context, actor domain.ID, in CreateDraftInput) (domain.Draft, error) {
	if _, err := s.store.Editor(ctx, actor); err != nil {
		return domain.Draft{}, err
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || len(in.Title) > 512 || !utf8.ValidString(in.Title) {
		return domain.Draft{}, domain.ErrInvalid
	}
	rendered, err := content.ValidateAndRender(in.Document)
	if err != nil {
		return domain.Draft{}, err
	}
	draft := domain.Draft{ID: domain.NewID(), ArticleID: domain.NewID(), OwnerID: actor, CategoryID: in.CategoryID, Title: in.Title, Document: in.Document, Version: 1, UpdatedAt: time.Now().Unix()}
	err = s.store.WithWrite(ctx, func(tx *sql.Tx) error {
		if err := storage.AssertActor(ctx, tx, actor, false); err != nil {
			return err
		}
		if in.CategoryID != "" {
			var count int
			if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM categories WHERE id=?", in.CategoryID).Scan(&count); err != nil {
				return err
			}
			if count != 1 {
				return domain.ErrInvalid
			}
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO articles(id,author_id) VALUES(?,?)", draft.ArticleID, actor); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO drafts(id,article_id,owner_id,category_id,title,document,base_article_version,updated_at) VALUES(?,?,?,?,?,?,0,?)", draft.ID, draft.ArticleID, actor, nullableID(draft.CategoryID), draft.Title, string(draft.Document), draft.UpdatedAt); err != nil {
			return err
		}
		return syncDraftMedia(ctx, tx, actor, draft.ID, rendered.MediaIDs)
	})
	if err != nil {
		return domain.Draft{}, err
	}
	return draft, nil
}
