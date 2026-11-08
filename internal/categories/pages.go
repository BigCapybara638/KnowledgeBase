package categories

import (
	"context"

	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
)

func (s *Service) ListPage(ctx context.Context, actor domain.ID, page int) ([]domain.Category, bool, error) {
	if _, err := s.store.Editor(ctx, actor); err != nil {
		return nil, false, err
	}
	if page < 1 || page > 100000 {
		return nil, false, domain.ErrInvalid
	}
	rows, err := s.store.Read.QueryContext(ctx, "SELECT id,title,sort_order,version FROM categories ORDER BY sort_order,title,id LIMIT 101 OFFSET ?", (page-1)*100)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := []domain.Category{}
	for rows.Next() {
		var item domain.Category
		if err := rows.Scan(&item.ID, &item.Title, &item.Order, &item.Version); err != nil {
			return nil, false, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasNext := len(items) > 100
	if hasNext {
		items = items[:100]
	}
	return items, hasNext, nil
}
