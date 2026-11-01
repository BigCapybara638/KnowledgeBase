package search

import (
	"context"
	"github.com/BigCapybara638/KnowledgeBase/internal/articles"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Service struct{ store *storage.Store }
type Item struct {
	ID                               domain.ID
	Title, Snippet, Category, Author string
}
type Page struct {
	Items   []Item
	Page    int
	HasNext bool
}

func New(s *storage.Store) *Service { return &Service{store: s} }
func (s *Service) Find(ctx context.Context, actor domain.ID, query string, page int) (Page, error) {
	if _, e := s.store.Editor(ctx, actor); e != nil {
		return Page{}, e
	}
	if page < 1 || page > 100000 || utf8.RuneCountInString(query) > 256 {
		return Page{}, domain.ErrInvalid
	}
	tokens := strings.FieldsFunc(query, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	p := Page{Page: page, Items: []Item{}}
	if len(tokens) == 0 {
		list, e := articles.New(s.store).List(ctx, actor, nil, page)
		if e != nil {
			return p, e
		}
		p.HasNext = list.HasNext
		for _, a := range list.Items {
			p.Items = append(p.Items, Item{ID: a.ID, Title: a.Title, Snippet: a.PlainText, Category: a.CategoryTitle, Author: a.AuthorName})
		}
		return p, nil
	}
	for i, v := range tokens {
		tokens[i] = `"` + v + `"`
	}
	match := strings.Join(tokens, " AND ")
	rows, e := s.store.Read.QueryContext(ctx, `SELECT a.id,a.title,snippet(articles_fts,1,'','',' … ',32),c.title,u.name FROM articles_fts JOIN articles a ON a.id=articles_fts.article_id JOIN categories c ON c.id=a.category_id JOIN users u ON u.id=a.author_id WHERE articles_fts MATCH ? AND a.published_at IS NOT NULL AND a.archived=0 ORDER BY bm25(articles_fts,10.0,1.0,0.0),a.id LIMIT 21 OFFSET ?`, match, (page-1)*20)
	if e != nil {
		return p, e
	}
	defer rows.Close()
	for rows.Next() {
		var i Item
		if e = rows.Scan(&i.ID, &i.Title, &i.Snippet, &i.Category, &i.Author); e != nil {
			return p, e
		}
		p.Items = append(p.Items, i)
	}
	if len(p.Items) > 20 {
		p.HasNext = true
		p.Items = p.Items[:20]
	}
	return p, rows.Err()
}
