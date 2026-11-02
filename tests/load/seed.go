// Test fixture seeding only; this package is never linked into the runtime image.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"golang.org/x/crypto/bcrypt"
	"strings"
	"time"
)

type seedResult struct {
	HotID      string `json:"hotId"`
	CategoryID string `json:"categoryId"`
	Articles   int    `json:"articles"`
}

func seedFixture(ctx context.Context, s *storage.Store, password string, count int) (seedResult, error) {
	hash, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if e != nil {
		return seedResult{}, e
	}
	tx, e := s.Write.BeginTx(ctx, nil)
	if e != nil {
		return seedResult{}, e
	}
	defer tx.Rollback()
	for _, login := range []string{"reader", "writer1", "writer2", "writer3", "writer4", "writer5"} {
		if _, e = tx.ExecContext(ctx, "INSERT INTO users(id,login,name,password_hash,role,must_change) VALUES(?,?,?,?, 'user',0)", domain.NewID(), login, login, hash); e != nil {
			return seedResult{}, e
		}
	}
	var admin string
	if e = tx.QueryRowContext(ctx, "SELECT id FROM users WHERE role='admin' LIMIT 1").Scan(&admin); e != nil {
		return seedResult{}, e
	}
	c := string(domain.NewID())
	if _, e = tx.ExecContext(ctx, "INSERT INTO categories(id,title) VALUES(?,'Нагрузочные статьи')", c); e != nil {
		return seedResult{}, e
	}
	result := seedResult{CategoryID: c, Articles: count}
	now := time.Now().Unix()
	for i := range count {
		id := string(domain.NewID())
		if i == 0 {
			result.HotID = id
		}
		title := fmt.Sprintf("Инструкция %05d", i)
		body := strings.Repeat("Правила работы команды и ответы на вопросы. ", 12)
		doc, _ := json.Marshal(map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": body}}}}})
		if _, e = tx.ExecContext(ctx, "INSERT INTO articles(id,category_id,author_id,last_editor_id,title,document,plain_text,version,published_at,updated_at) VALUES(?,?,?,?,?,?,?,1,?,?)", id, c, admin, admin, title, string(doc), body, now, now); e != nil {
			return seedResult{}, e
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO articles_fts(article_id,title,body) VALUES(?,?,?)", id, title, body); e != nil {
			return seedResult{}, e
		}
	}
	return result, tx.Commit()
}
