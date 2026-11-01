package categories_test

import (
	"context"
	"errors"
	"github.com/BigCapybara638/KnowledgeBase/internal/categories"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/testutil"
	"testing"
)

func TestOnlyAdminManagesCategories(t *testing.T) {
	s := testutil.Store(t)
	u := testutil.User(t, s, "user")
	_, e := categories.New(s).Save(context.Background(), u, categories.Update{Title: "Инструкции"})
	if !errors.Is(e, domain.ErrForbidden) {
		t.Fatal(e)
	}
}
func TestDeletingNonemptyCategoryFails(t *testing.T) {
	s := testutil.Store(t)
	admin := testutil.User(t, s, "admin")
	c := categories.New(s)
	item, e := c.Save(context.Background(), admin, categories.Update{Title: "Инструкции"})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Write.Exec("INSERT INTO articles(id,category_id,author_id) VALUES('a',?,?)", item.ID, admin)
	if e != nil {
		t.Fatal(e)
	}
	if e = c.Delete(context.Background(), admin, item.ID, item.Version); !errors.Is(e, domain.ErrConflict) {
		t.Fatalf("nonempty category deleted: %v", e)
	}
	rows, e := c.List(context.Background())
	if e != nil || len(rows) != 1 {
		t.Fatalf("category lost: %v", e)
	}
}
