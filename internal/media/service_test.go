package media_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"github.com/BigCapybara638/KnowledgeBase/internal/articles"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/media"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"github.com/BigCapybara638/KnowledgeBase/internal/testutil"
	"hash/crc32"
	"image"
	"image/png"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) (*storage.Store, *media.Service, domain.ID, domain.Draft) {
	t.Helper()
	s := testutil.Store(t)
	u := testutil.User(t, s, "user")
	d, e := articles.New(s).CreateDraft(context.Background(), u, nil)
	if e != nil {
		t.Fatal(e)
	}
	return s, media.New(s), u, d
}
func picture() []byte {
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	return b.Bytes()
}
func upload(t *testing.T, m *media.Service, u domain.ID, d domain.Draft) domain.Media {
	t.Helper()
	v, e := m.Upload(context.Background(), u, d.ID, bytes.NewReader(picture()), "photo.png")
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func imageDoc(id domain.ID) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"type": "doc", "content": []any{map[string]any{"type": "image", "attrs": map[string]any{"src": "/media/" + string(id), "alt": "Изображение"}}}})
	return b
}
func TestPrivateDraftMediaDeniedToOtherUser(t *testing.T) {
	s, m, u, d := fixture(t)
	v := upload(t, m, u, d)
	other := testutil.User(t, s, "user")
	if f, e := m.Open(context.Background(), other, v.ID); !errors.Is(e, domain.ErrNotFound) {
		if e == nil {
			f.Close()
		}
		t.Fatalf("private media leaked: %v", e)
	}
	f, e := m.Open(context.Background(), u, v.ID)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	b, _ := io.ReadAll(f)
	if !bytes.Equal(b, picture()) {
		t.Fatal("image damaged")
	}
}
func TestPublishedMediaRequiresLogin(t *testing.T) {
	s, m, u, d := fixture(t)
	v := upload(t, m, u, d)
	c := domain.NewID()
	s.Write.Exec("INSERT INTO categories(id,title) VALUES(?,'Медиа')", c)
	a := articles.New(s)
	d, e := a.SaveDraft(context.Background(), articles.SaveDraftInput{DraftID: d.ID, ActorID: u, CategoryID: c, Title: "Картинка", Document: imageDoc(v.ID), ExpectedVersion: d.Version})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = a.Publish(context.Background(), u, d.ID, d.Version, d.BaseArticleVersion); e != nil {
		t.Fatal(e)
	}
	other := testutil.User(t, s, "user")
	f, e := m.Open(context.Background(), other, v.ID)
	if e != nil {
		t.Fatal(e)
	}
	f.Close()
	if f, e = m.Open(context.Background(), "", v.ID); e == nil {
		f.Close()
		t.Fatal("guest image access")
	}
}
func TestCannotPublishSomeoneElsesPrivateImage(t *testing.T) {
	s, m, u, d := fixture(t)
	v := upload(t, m, u, d)
	other := testutil.User(t, s, "user")
	a := articles.New(s)
	od, e := a.CreateDraft(context.Background(), other, nil)
	if e != nil {
		t.Fatal(e)
	}
	_, e = a.SaveDraft(context.Background(), articles.SaveDraftInput{DraftID: od.ID, ActorID: other, ExpectedVersion: od.Version, Document: imageDoc(v.ID)})
	if !errors.Is(e, domain.ErrForbidden) {
		t.Fatalf("private image reference accepted: %v", e)
	}
}
func TestMimeSpoofAndTraversalRejected(t *testing.T) {
	_, m, u, d := fixture(t)
	for _, v := range []struct {
		body []byte
		name string
	}{{[]byte("<script>alert(1)</script>"), "fake.png"}, {picture(), "../../outside.png"}} {
		if _, e := m.Upload(context.Background(), u, d.ID, bytes.NewReader(v.body), v.name); !errors.Is(e, domain.ErrInvalid) {
			t.Fatal(e)
		}
	}
}
func TestSizeAndPixelLimits(t *testing.T) {
	_, m, u, d := fixture(t)
	if _, e := m.Upload(context.Background(), u, d.ID, strings.NewReader(strings.Repeat("x", (10<<20)+1)), "big.png"); !errors.Is(e, domain.ErrInvalid) {
		t.Fatal(e)
	}
	b := picture()
	binary.BigEndian.PutUint32(b[16:20], 50000)
	binary.BigEndian.PutUint32(b[20:24], 50000)
	binary.BigEndian.PutUint32(b[29:33], crc32.ChecksumIEEE(b[12:29]))
	if _, e := m.Upload(context.Background(), u, d.ID, bytes.NewReader(b), "pixels.png"); !errors.Is(e, domain.ErrInvalid) {
		t.Fatal(e)
	}
}

type blocked struct {
	entered chan<- struct{}
	release <-chan struct{}
}

func (b blocked) Read(p []byte) (int, error) { b.entered <- struct{}{}; <-b.release; return 0, io.EOF }
func TestThirdParallelUploadRejected(t *testing.T) {
	_, m, u, d := fixture(t)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	done := make(chan struct{}, 2)
	defer func() {
		close(release)
		for range 2 {
			<-done
		}
	}()
	for range 2 {
		go func() {
			m.Upload(context.Background(), u, d.ID, blocked{entered, release}, "wait.png")
			done <- struct{}{}
		}()
	}
	for range 2 {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("uploads did not start")
		}
	}
	if _, e := m.Upload(context.Background(), u, d.ID, bytes.NewReader(picture()), "third.png"); !errors.Is(e, media.ErrUploadLimit) {
		t.Fatal(e)
	}
}

type broken struct{}

func (broken) Read([]byte) (int, error) { return 0, errors.New("read failure") }
func TestFailedUploadLeavesNoPublishedReference(t *testing.T) {
	s, m, u, d := fixture(t)
	if _, e := m.Upload(context.Background(), u, d.ID, broken{}, "broken.png"); e == nil {
		t.Fatal("broken source accepted")
	}
	var n int
	s.Read.QueryRow("SELECT count(*) FROM media").Scan(&n)
	if n != 0 {
		t.Fatal("failed upload persisted")
	}
}
func TestGCRespectsDraftAndArticleReferences(t *testing.T) {
	s, m, u, d := fixture(t)
	v := upload(t, m, u, d)
	s.Write.Exec("UPDATE media SET created_at=1")
	n, e := m.CollectOrphans(context.Background(), time.Now().Add(-24*time.Hour))
	if e != nil || n != 0 {
		t.Fatal(n, e)
	}
	a := articles.New(s)
	d, e = a.SaveDraft(context.Background(), articles.SaveDraftInput{DraftID: d.ID, ActorID: u, ExpectedVersion: d.Version, Document: []byte(`{"type":"doc","content":[{"type":"paragraph"}]}`)})
	if e != nil {
		t.Fatal(e)
	}
	n, e = m.CollectOrphans(context.Background(), time.Now().Add(-24*time.Hour))
	if e != nil || n != 1 {
		t.Fatal(n, e)
	}
	if _, e = m.Open(context.Background(), u, v.ID); !errors.Is(e, domain.ErrNotFound) {
		t.Fatal(e)
	}
}

func TestDraftReferencePreservesAccessAfterPublishedImageRemoval(t *testing.T) {
	s, m, u, d := fixture(t)
	v := upload(t, m, u, d)
	c := domain.NewID()
	s.Write.Exec("INSERT INTO categories(id,title) VALUES(?,'Медиа')", c)
	a := articles.New(s)
	d, e := a.SaveDraft(context.Background(), articles.SaveDraftInput{DraftID: d.ID, ActorID: u, CategoryID: c, Title: "Фото", Document: imageDoc(v.ID), ExpectedVersion: d.Version})
	if e != nil {
		t.Fatal(e)
	}
	pub, e := a.Publish(context.Background(), u, d.ID, d.Version, d.BaseArticleVersion)
	if e != nil {
		t.Fatal(e)
	}
	other := testutil.User(t, s, "user")
	kept, e := a.CreateDraft(context.Background(), other, &pub.ID)
	if e != nil {
		t.Fatal(e)
	}
	remove, e := a.CreateDraft(context.Background(), u, &pub.ID)
	if e != nil {
		t.Fatal(e)
	}
	remove, e = a.SaveDraft(context.Background(), articles.SaveDraftInput{DraftID: remove.ID, ActorID: u, CategoryID: c, Title: "Без фото", Document: []byte(`{"type":"doc","content":[{"type":"paragraph"}]}`), ExpectedVersion: remove.Version})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = a.Publish(context.Background(), u, remove.ID, remove.Version, remove.BaseArticleVersion); e != nil {
		t.Fatal(e)
	}
	f, e := m.Open(context.Background(), other, v.ID)
	if e != nil {
		t.Errorf("image from own saved draft became unavailable: %v", e)
	} else {
		f.Close()
	}
	if _, e = a.SaveDraft(context.Background(), articles.SaveDraftInput{DraftID: kept.ID, ActorID: other, CategoryID: c, Title: kept.Title, Document: kept.Document, ExpectedVersion: kept.Version}); e != nil {
		t.Errorf("own draft cannot be saved after image removed from publication: %v", e)
	}
}

func TestVideoUploadAndPublishRetainsProtection(t *testing.T) {
	s, m, u, d := fixture(t)
	data, err := os.ReadFile("../../tests/e2e/fixtures/clip.webm")
	if err != nil {
		t.Fatal(err)
	}
	video, err := m.Upload(context.Background(), u, d.ID, bytes.NewReader(data), "clip.webm")
	if err != nil {
		t.Fatal(err)
	}
	if video.Type != "video/webm" {
		t.Fatal(video.Type)
	}
	other := testutil.User(t, s, "user")
	if f, err := m.Open(context.Background(), other, video.ID); err == nil {
		f.Close()
		t.Fatal("private video leaked")
	}
	category := domain.NewID()
	if _, err = s.Write.Exec("INSERT INTO categories(id,title) VALUES(?,'Видео')", category); err != nil {
		t.Fatal(err)
	}
	doc, _ := json.Marshal(map[string]any{"type": "doc", "content": []any{map[string]any{"type": "video", "attrs": map[string]any{"src": "/media/" + string(video.ID), "title": "Обучение"}}}})
	service := articles.New(s)
	d, err = service.SaveDraft(context.Background(), articles.SaveDraftInput{DraftID: d.ID, ActorID: u, CategoryID: category, Title: "Видео", Document: doc, ExpectedVersion: d.Version})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Publish(context.Background(), u, d.ID, d.Version, d.BaseArticleVersion); err != nil {
		t.Fatal(err)
	}
	f, err := m.Open(context.Background(), other, video.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if f, err := m.Open(context.Background(), "", video.ID); err == nil {
		f.Close()
		t.Fatal("guest video leaked")
	}
}
