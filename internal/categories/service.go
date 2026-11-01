package categories

import (
	"context"
	"database/sql"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"strings"
)

type Service struct{ store *storage.Store }
type Update struct {
	ID              domain.ID
	Title           string
	Order           int
	ExpectedVersion domain.Version
}

func New(s *storage.Store) *Service { return &Service{store: s} }
func (s *Service) List(ctx context.Context) ([]domain.Category, error) {
	rows, e := s.store.Read.QueryContext(ctx, "SELECT id,title,sort_order,version FROM categories ORDER BY sort_order,title,id")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.Category{}
	for rows.Next() {
		var c domain.Category
		if e = rows.Scan(&c.ID, &c.Title, &c.Order, &c.Version); e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Service) Save(ctx context.Context, admin domain.ID, in Update) (domain.Category, error) {
	if e := s.store.Admin(ctx, admin); e != nil {
		return domain.Category{}, e
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || len(in.Title) > 256 {
		return domain.Category{}, domain.ErrInvalid
	}
	item := domain.Category{ID: in.ID, Title: in.Title, Order: in.Order, Version: in.ExpectedVersion + 1}
	fresh := in.ID == ""
	if fresh {
		item.ID = domain.NewID()
		item.Version = 1
	}
	e := s.store.WithWrite(ctx, func(tx *sql.Tx) error {
		if e := storage.AssertActor(ctx, tx, admin, true); e != nil {
			return e
		}
		if fresh {
			_, e := tx.ExecContext(ctx, "INSERT INTO categories(id,title,sort_order) VALUES(?,?,?)", item.ID, item.Title, item.Order)
			return e
		}
		r, e := tx.ExecContext(ctx, "UPDATE categories SET title=?,sort_order=?,version=version+1 WHERE id=? AND version=?", item.Title, item.Order, item.ID, in.ExpectedVersion)
		if e != nil {
			return e
		}
		n, _ := r.RowsAffected()
		if n != 1 {
			return domain.ErrConflict
		}
		return nil
	})
	return item, e
}
func (s *Service) Delete(ctx context.Context, admin, id domain.ID, expected domain.Version) error {
	if e := s.store.Admin(ctx, admin); e != nil {
		return e
	}
	return s.store.WithWrite(ctx, func(tx *sql.Tx) error {
		if e := storage.AssertActor(ctx, tx, admin, true); e != nil {
			return e
		}
		var count int
		if e := tx.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM articles WHERE category_id=?)+(SELECT count(*) FROM drafts WHERE category_id=?)", id, id).Scan(&count); e != nil {
			return e
		}
		if count > 0 {
			return domain.ErrConflict
		}
		r, e := tx.ExecContext(ctx, "DELETE FROM categories WHERE id=? AND version=?", id, expected)
		if e != nil {
			return e
		}
		n, _ := r.RowsAffected()
		if n != 1 {
			return domain.ErrConflict
		}
		return nil
	})
}
