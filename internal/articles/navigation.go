package articles

import (
	"context"
	"database/sql"
	"errors"

	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
)

type ArticleNeighbors struct {
	Previous domain.Article
	Next     domain.Article
}

func (s *Service) Neighbors(ctx context.Context, actor, id domain.ID) (ArticleNeighbors, error) {
	item, err := s.Get(ctx, actor, id)
	if err != nil {
		return ArticleNeighbors{}, err
	}
	var neighbors ArticleNeighbors
	for _, direction := range []struct {
		comparison string
		order      string
		target     *domain.Article
	}{
		{"<", "DESC", &neighbors.Previous},
		{">", "ASC", &neighbors.Next},
	} {
		query := `SELECT id,title FROM articles WHERE category_id=? AND published_at IS NOT NULL AND archived=0
			AND (title COLLATE NOCASE,id) ` + direction.comparison + ` (?,?)
			ORDER BY title COLLATE NOCASE ` + direction.order + `,id ` + direction.order + ` LIMIT 1`
		err := s.store.Read.QueryRowContext(ctx, query, item.CategoryID, item.Title, item.ID).Scan(&direction.target.ID, &direction.target.Title)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return ArticleNeighbors{}, err
		}
	}
	return neighbors, nil
}
