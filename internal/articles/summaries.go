package articles

import (
	"context"

	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
)

// Summary omits the full document so a page remains small even for large articles.
type Summary struct {
	ID                               domain.ID
	Title, Snippet, Category, Author string
}

func (s *Service) ListSummaries(ctx context.Context, actor domain.ID, category *domain.ID, page int) ([]Summary, bool, error) {
	if _, err := s.store.Editor(ctx, actor); err != nil {
		return nil, false, err
	}
	if page < 1 || page > 100000 {
		return nil, false, domain.ErrInvalid
	}
	q := `SELECT a.id,a.title,substr(a.plain_text,1,240),c.title,u.name FROM articles a JOIN categories c ON c.id=a.category_id JOIN users u ON u.id=a.author_id WHERE a.published_at IS NOT NULL AND a.archived=0`
	args := []any{}
	if category != nil {
		q += " AND a.category_id=?"
		args = append(args, *category)
	}
	q += " ORDER BY a.updated_at DESC,a.id LIMIT 21 OFFSET ?"
	args = append(args, (page-1)*20)
	rows, err := s.store.Read.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := []Summary{}
	for rows.Next() {
		var item Summary
		if err := rows.Scan(&item.ID, &item.Title, &item.Snippet, &item.Category, &item.Author); err != nil {
			return nil, false, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasNext := len(items) > 20
	if hasNext {
		items = items[:20]
	}
	return items, hasNext, nil
}
