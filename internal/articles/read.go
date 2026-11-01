package articles

import (
	"context"
	"database/sql"
	"errors"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"strconv"
)

type Service struct{ store *storage.Store }
type Page struct {
	Items   []domain.Article
	Page    int
	HasNext bool
}
type MenuPage struct {
	Items []domain.Article
	Next  string
}

func New(s *storage.Store) *Service { return &Service{store: s} }

const publishedSelect = `SELECT a.id,a.category_id,a.author_id,a.last_editor_id,a.title,a.document,a.plain_text,a.version,a.archived,a.published_at,a.updated_at,u.name,e.name,c.title FROM articles a JOIN users u ON u.id=a.author_id JOIN users e ON e.id=a.last_editor_id JOIN categories c ON c.id=a.category_id `

type scanner interface{ Scan(...any) error }

func scanArticle(row scanner) (domain.Article, error) {
	var a domain.Article
	var doc string
	e := row.Scan(&a.ID, &a.CategoryID, &a.AuthorID, &a.LastEditorID, &a.Title, &doc, &a.PlainText, &a.Version, &a.Archived, &a.PublishedAt, &a.UpdatedAt, &a.AuthorName, &a.EditorName, &a.CategoryTitle)
	a.Document = []byte(doc)
	if errors.Is(e, sql.ErrNoRows) {
		e = domain.ErrNotFound
	}
	return a, e
}
func (s *Service) Get(ctx context.Context, actor, id domain.ID) (domain.Article, error) {
	if _, e := s.store.Editor(ctx, actor); e != nil {
		return domain.Article{}, e
	}
	return scanArticle(s.store.Read.QueryRowContext(ctx, publishedSelect+"WHERE a.id=? AND a.published_at IS NOT NULL AND a.archived=0", id))
}
func (s *Service) List(ctx context.Context, actor domain.ID, category *domain.ID, page int) (Page, error) {
	if _, e := s.store.Editor(ctx, actor); e != nil {
		return Page{}, e
	}
	if page < 1 || page > 100000 {
		return Page{}, domain.ErrInvalid
	}
	q := publishedSelect + "WHERE a.published_at IS NOT NULL AND a.archived=0"
	args := []any{}
	if category != nil {
		q += " AND a.category_id=?"
		args = append(args, *category)
	}
	q += " ORDER BY a.updated_at DESC,a.id LIMIT 21 OFFSET ?"
	args = append(args, (page-1)*20)
	rows, e := s.store.Read.QueryContext(ctx, q, args...)
	if e != nil {
		return Page{}, e
	}
	defer rows.Close()
	p := Page{Page: page, Items: []domain.Article{}}
	for rows.Next() {
		a, e := scanArticle(rows)
		if e != nil {
			return Page{}, e
		}
		p.Items = append(p.Items, a)
	}
	if len(p.Items) > 20 {
		p.HasNext = true
		p.Items = p.Items[:20]
	}
	return p, rows.Err()
}
func (s *Service) ListCategoryMenu(ctx context.Context, actor, category domain.ID, cursor string) (MenuPage, error) {
	if _, e := s.store.Editor(ctx, actor); e != nil {
		return MenuPage{}, e
	}
	offset := 0
	if cursor != "" {
		var e error
		offset, e = strconv.Atoi(cursor)
		if e != nil || offset < 0 || offset > 1000000 {
			return MenuPage{}, domain.ErrInvalid
		}
	}
	rows, e := s.store.Read.QueryContext(ctx, "SELECT id,title FROM articles WHERE category_id=? AND published_at IS NOT NULL AND archived=0 ORDER BY title COLLATE NOCASE,id LIMIT 51 OFFSET ?", category, offset)
	if e != nil {
		return MenuPage{}, e
	}
	defer rows.Close()
	p := MenuPage{Items: []domain.Article{}}
	for rows.Next() {
		var a domain.Article
		if e = rows.Scan(&a.ID, &a.Title); e != nil {
			return p, e
		}
		p.Items = append(p.Items, a)
	}
	if len(p.Items) > 50 {
		p.Items = p.Items[:50]
		p.Next = strconv.Itoa(offset + 50)
	}
	return p, rows.Err()
}
