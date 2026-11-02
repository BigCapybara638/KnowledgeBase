package main

import (
	"context"
	"github.com/BigCapybara638/KnowledgeBase/internal/testutil"
	"testing"
)

func TestSeedCreates5000IndexedArticlesAndDistinctUsers(t *testing.T) {
	s := testutil.Store(t)
	testutil.User(t, s, "admin")
	result, e := seedFixture(context.Background(), s, testutil.Password, 5000)
	if e != nil {
		t.Fatal(e)
	}
	var articles, indexed, users int
	s.Read.QueryRow("SELECT count(*) FROM articles WHERE published_at IS NOT NULL").Scan(&articles)
	s.Read.QueryRow("SELECT count(*) FROM articles_fts").Scan(&indexed)
	s.Read.QueryRow("SELECT count(*) FROM users").Scan(&users)
	if articles != 5000 || indexed != 5000 || users != 7 || result.HotID == "" {
		t.Fatal(articles, indexed, users, result)
	}
}
