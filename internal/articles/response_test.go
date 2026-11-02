package articles_test

import (
	"context"
	"database/sql"
	"github.com/BigCapybara638/KnowledgeBase/internal/articles"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"testing"
	"time"
)

func await(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !fn() {
		if time.Now().After(deadline) {
			t.Fatal("synchronization deadline exceeded")
		}
		time.Sleep(time.Millisecond)
	}
}
func occupyReaders(t *testing.T, s *storage.Store) []*sql.Conn {
	t.Helper()
	out := []*sql.Conn{}
	for range 4 {
		c, e := s.Read.Conn(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		out = append(out, c)
	}
	return out
}
func TestSaveResponseKeepsItsOwnCommittedVersion(t *testing.T) {
	s, u, c := seed(t, 0)
	d := edit(t, s, u, c, nil, "Статья", "Исходное")
	tx, e := s.Write.BeginTx(context.Background(), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	before := s.Write.Stats().WaitCount
	done := make(chan struct {
		d domain.Draft
		e error
	}, 1)
	go func() {
		v, e := articles.New(s).SaveDraft(context.Background(), articles.SaveDraftInput{DraftID: d.ID, ActorID: u, CategoryID: c, ExpectedVersion: d.Version, Title: "Моя правка", Document: document("Мой текст")})
		done <- struct {
			d domain.Draft
			e error
		}{v, e}
	}()
	await(t, func() bool { return s.Write.Stats().WaitCount > before })
	readers := occupyReaders(t, s)
	defer func() {
		for _, r := range readers {
			r.Close()
		}
	}()
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	await(t, func() bool { return s.Write.Stats().InUse == 0 })
	if _, e = s.Write.Exec("UPDATE drafts SET version=version+1,title='Следующая правка' WHERE id=?", d.ID); e != nil {
		t.Fatal(e)
	}
	for _, r := range readers {
		r.Close()
	}
	r := <-done
	if r.e != nil {
		t.Fatal(r.e)
	}
	if r.d.Version != d.Version+1 || r.d.Title != "Моя правка" {
		t.Fatalf("later writer leaked into response: %+v", r.d)
	}
}
func TestPublishResponseKeepsItsOwnCommittedVersion(t *testing.T) {
	s, u, c := seed(t, 1)
	id := domain.ID("article-000")
	d := edit(t, s, u, c, &id, "Моя публикация", "Мой текст")
	tx, e := s.Write.BeginTx(context.Background(), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	before := s.Write.Stats().WaitCount
	done := make(chan struct {
		a domain.Article
		e error
	}, 1)
	go func() {
		a, e := articles.New(s).Publish(context.Background(), u, d.ID, d.Version, d.BaseArticleVersion)
		done <- struct {
			a domain.Article
			e error
		}{a, e}
	}()
	await(t, func() bool { return s.Write.Stats().WaitCount > before })
	readers := occupyReaders(t, s)
	defer func() {
		for _, r := range readers {
			r.Close()
		}
	}()
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	await(t, func() bool { return s.Write.Stats().InUse == 0 })
	if _, e = s.Write.Exec("UPDATE articles SET version=version+1,title='Следующая публикация' WHERE id=?", id); e != nil {
		t.Fatal(e)
	}
	for _, r := range readers {
		r.Close()
	}
	r := <-done
	if r.e != nil {
		t.Fatal(r.e)
	}
	if r.a.Version != d.BaseArticleVersion+1 || r.a.Title != "Моя публикация" {
		t.Fatalf("later publication leaked into response: %+v", r.a)
	}
}
