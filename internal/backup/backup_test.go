package backup_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/BigCapybara638/KnowledgeBase/internal/articles"
	"github.com/BigCapybara638/KnowledgeBase/internal/auth"
	"github.com/BigCapybara638/KnowledgeBase/internal/backup"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/media"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"github.com/BigCapybara638/KnowledgeBase/internal/testutil"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixture(t *testing.T) (*storage.Store, domain.ID, domain.ID) {
	t.Helper()
	s := testutil.Store(t)
	u := testutil.User(t, s, "admin")
	a, e := auth.New(s, auth.Config{})
	if e != nil {
		t.Fatal(e)
	}
	_ = a
	c := domain.NewID()
	if _, e = s.Write.Exec("INSERT INTO categories(id,title) VALUES(?,'Инструкции')", c); e != nil {
		t.Fatal(e)
	}
	svc := articles.New(s)
	d, e := svc.CreateDraft(context.Background(), u, nil)
	if e != nil {
		t.Fatal(e)
	}
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	m, e := media.New(s).Upload(context.Background(), u, d.ID, &b, "test.png")
	if e != nil {
		t.Fatal(e)
	}
	doc, _ := json.Marshal(map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": "Сохранённый текст"}}}, map[string]any{"type": "image", "attrs": map[string]any{"src": "/media/" + string(m.ID)}}}})
	d, e = svc.SaveDraft(context.Background(), articles.SaveDraftInput{DraftID: d.ID, ActorID: u, CategoryID: c, Title: "Публикация", ExpectedVersion: d.Version, Document: doc})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Publish(context.Background(), u, d.ID, d.Version, d.BaseArticleVersion); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.CreateDraft(context.Background(), u, nil); e != nil {
		t.Fatal(e)
	}
	return s, u, m.ID
}
func makeCopy(t *testing.T, s *storage.Store, dir string) backup.Manifest {
	t.Helper()
	m, e := backup.Create(context.Background(), s.DataDir, dir)
	if e != nil {
		t.Fatal(e)
	}
	return m
}
func restore(t *testing.T, m backup.Manifest) *storage.Store {
	t.Helper()
	d := t.TempDir()
	if e := backup.Restore(context.Background(), m.Directory, d); e != nil {
		t.Fatal(e)
	}
	s, e := storage.Open(context.Background(), d)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func TestBackupIncludesCommittedWAL(t *testing.T) {
	s, _, _ := fixture(t)
	s.Write.Exec("PRAGMA wal_autocheckpoint=0")
	s.Write.Exec("INSERT INTO categories(id,title) VALUES('wal','Из WAL')")
	m := makeCopy(t, s, t.TempDir())
	r := restore(t, m)
	var n int
	r.Read.QueryRow("SELECT count(*) FROM categories WHERE id='wal'").Scan(&n)
	if n != 1 {
		t.Fatal("committed WAL lost")
	}
}
func TestBackupDuringPublishAndGCIsRestorable(t *testing.T) {
	s, u, _ := fixture(t)
	root := t.TempDir()
	ctx := context.Background()
	start := make(chan struct{})
	out := make(chan error, 3)
	var m backup.Manifest
	var wg sync.WaitGroup
	wg.Go(func() { <-start; var e error; m, e = backup.Create(ctx, s.DataDir, root); out <- e })
	wg.Go(func() { <-start; _, e := media.New(s).CollectOrphans(ctx, time.Now().Add(-24*time.Hour)); out <- e })
	wg.Go(func() {
		<-start
		for range 5 {
			d, e := articles.New(s).CreateDraft(ctx, u, nil)
			if e != nil {
				out <- e
				return
			}
			_, e = articles.New(s).SaveDraft(ctx, articles.SaveDraftInput{DraftID: d.ID, ActorID: u, ExpectedVersion: d.Version, Title: "Параллельный черновик", Document: []byte(`{"type":"doc","content":[{"type":"paragraph"}]}`)})
			if e != nil {
				out <- e
				return
			}
		}
		out <- nil
	})
	close(start)
	wg.Wait()
	close(out)
	for e := range out {
		if e != nil {
			t.Fatal(e)
		}
	}
	r := restore(t, m)
	var result string
	if e := r.Read.QueryRow("PRAGMA integrity_check").Scan(&result); e != nil || result != "ok" {
		t.Fatal(result, e)
	}
	var id string
	r.Read.QueryRow("SELECT id FROM media LIMIT 1").Scan(&id)
	if _, e := os.Stat(filepath.Join(r.DataDir, "media", id)); e != nil {
		t.Fatal(e)
	}
}
func TestRestoreRejectsRunningApp(t *testing.T) {
	s, _, _ := fixture(t)
	m := makeCopy(t, s, t.TempDir())
	lock, e := storage.AcquireInstanceLock(s.DataDir)
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Close()
	if e = backup.Restore(context.Background(), m.Directory, s.DataDir); e == nil || !strings.Contains(e.Error(), "in use") {
		t.Fatalf("running app accepted: %v", e)
	}
}
func TestRestoreRejectsNonemptyTarget(t *testing.T) {
	s, _, _ := fixture(t)
	m := makeCopy(t, s, t.TempDir())
	target := t.TempDir()
	p := filepath.Join(target, "keep.txt")
	os.WriteFile(p, []byte("keep"), 0600)
	if e := backup.Restore(context.Background(), m.Directory, target); e == nil {
		t.Fatal("nonempty target accepted")
	}
	b, _ := os.ReadFile(p)
	if string(b) != "keep" {
		t.Fatal("user file overwritten")
	}
}
func TestFailedCopyKeepsPreviousBackups(t *testing.T) {
	s, _, id := fixture(t)
	root := t.TempDir()
	m := makeCopy(t, s, root)
	p := filepath.Join(s.DataDir, "media", string(id))
	if e := os.Rename(p, p+".hidden"); e != nil {
		t.Fatal(e)
	}
	defer os.Rename(p+".hidden", p)
	if _, e := backup.Create(context.Background(), s.DataDir, root); e == nil {
		t.Fatal("missing referenced file accepted")
	}
	if _, e := os.Stat(filepath.Join(m.Directory, "manifest.json")); e != nil {
		t.Fatal("previous copy deleted", e)
	}
}
func TestRotateKeeps14AfterSuccessfulCopy(t *testing.T) {
	s, _, _ := fixture(t)
	root := t.TempDir()
	first := makeCopy(t, s, root)
	for range 14 {
		makeCopy(t, s, root)
	}
	entries, _ := os.ReadDir(root)
	count := 0
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "kb-") {
			count++
		}
	}
	if count != 14 {
		t.Fatal(count)
	}
	if _, e := os.Stat(first.Directory); !os.IsNotExist(e) {
		t.Fatal("oldest copy retained")
	}
}
func TestRestoreRevokesSessions(t *testing.T) {
	s, u, _ := fixture(t)
	a, _ := auth.New(s, auth.Config{})
	user, _ := s.User(context.Background(), u)
	l, e := a.Login(context.Background(), user.Login, testutil.Password)
	if e != nil {
		t.Fatal(e)
	}
	m := makeCopy(t, s, t.TempDir())
	r := restore(t, m)
	b, e := auth.New(r, auth.Config{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = b.ResolveSession(context.Background(), l.RawToken); !errors.Is(e, domain.ErrForbidden) {
		t.Fatal("old session restored", e)
	}
	if _, e = b.Login(context.Background(), user.Login, testutil.Password); e != nil {
		t.Fatal("new login failed", e)
	}
	var n int
	r.Read.QueryRow("SELECT count(*) FROM drafts").Scan(&n)
	if n != 1 {
		t.Fatal("private draft not restored")
	}
}
func TestChecksumMismatchRejectsBeforeReplacement(t *testing.T) {
	s, _, _ := fixture(t)
	m := makeCopy(t, s, t.TempDir())
	f, e := os.OpenFile(filepath.Join(m.Directory, "knowledgebase.db"), os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	f.Write([]byte("tampered"))
	f.Close()
	target := t.TempDir()
	if e = backup.Restore(context.Background(), m.Directory, target); e == nil || !strings.Contains(e.Error(), "checksum") {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(target, "knowledgebase.db")); !os.IsNotExist(e) {
		t.Fatal("invalid copy installed")
	}
}
func TestIncompleteRestorePreventsStartup(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "restore-in-progress"), []byte("pending"), 0600)
	if s, e := storage.Open(context.Background(), d); e == nil {
		s.Close()
		t.Fatal("incomplete restore accepted")
	}
}
