package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"hash/crc32"
	"html"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type credentials struct {
	Password string `json:"password"`
}

func main() {
	if e := mainRun(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func mainRun() error {
	if len(os.Args) < 2 {
		return errors.New("seed or run required")
	}
	f := flag.NewFlagSet(os.Args[1], flag.ContinueOnError)
	dir := f.String("data-dir", "/data", "")
	origin := f.String("url", "", "")
	cert := f.String("ca", "", "")
	duration := f.Duration("duration", 10*time.Minute, "")
	hot := f.String("hot-id", "", "")
	category := f.String("category-id", "", "")
	if e := f.Parse(os.Args[2:]); e != nil {
		return e
	}
	var cred credentials
	if e := json.NewDecoder(io.LimitReader(os.Stdin, 4096)).Decode(&cred); e != nil {
		return e
	}
	if len(cred.Password) < 12 || len(cred.Password) > 72 {
		return errors.New("invalid fixture password")
	}
	if os.Args[1] == "seed" {
		s, e := storage.Open(context.Background(), *dir)
		if e != nil {
			return e
		}
		defer s.Close()
		r, e := seedFixture(context.Background(), s, cred.Password, 5000)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(r)
	}
	if os.Args[1] != "run" || *duration < time.Second || *hot == "" {
		return errors.New("invalid run arguments")
	}
	return exercise(*origin, *cert, cred.Password, *hot, *category, *duration)
}

type client struct {
	origin, csrf string
	http         *http.Client
}

var tokenRE = regexp.MustCompile(`<meta name="csrf-token" content="([^"]+)"`)
var stateRE = regexp.MustCompile(`(?s)id="editor-state">(.*?)</script>`)

func newClient(origin, cert, login, password string) (*client, error) {
	b, e := os.ReadFile(cert)
	if e != nil {
		return nil, e
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(b) {
		return nil, errors.New("invalid test CA")
	}
	jar, _ := cookiejar.New(nil)
	c := &client{origin: origin, http: &http.Client{Timeout: 15 * time.Second, Jar: jar, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}, MaxIdleConns: 100, MaxIdleConnsPerHost: 64}}}
	status, body, e := c.request(context.Background(), "GET", "/login", nil)
	if e != nil || status != 200 {
		return nil, fmt.Errorf("login form: %d %v", status, e)
	}
	m := tokenRE.FindSubmatch(body)
	if len(m) != 2 {
		return nil, errors.New("missing CSRF token")
	}
	c.csrf = html.UnescapeString(string(m[1]))
	status, _, e = c.request(context.Background(), "POST", "/login", url.Values{"login": {login}, "password": {password}, "csrf_token": {c.csrf}})
	if e != nil || status != 200 {
		return nil, fmt.Errorf("login failed: %d %v", status, e)
	}
	return c, nil
}
func (c *client) request(ctx context.Context, method, path string, payload any) (int, []byte, error) {
	var body io.Reader
	typ := ""
	if payload != nil {
		if form, ok := payload.(url.Values); ok {
			body = strings.NewReader(form.Encode())
			typ = "application/x-www-form-urlencoded"
		} else {
			b, e := json.Marshal(payload)
			if e != nil {
				return 0, nil, e
			}
			body = bytes.NewReader(b)
			typ = "application/json"
		}
	}
	req, e := http.NewRequestWithContext(ctx, method, c.origin+path, body)
	if e != nil {
		return 0, nil, e
	}
	req.Header.Set("Content-Type", typ)
	req.Header.Set("Origin", c.origin)
	req.Header.Set("Referer", c.origin+"/")
	req.Header.Set("X-CSRF-Token", c.csrf)
	if typ == "application/json" {
		req.Header.Set("Accept", "application/json")
	}
	r, e := c.http.Do(req)
	if e != nil {
		return 0, nil, e
	}
	defer r.Body.Close()
	b, e := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	return r.StatusCode, b, e
}

type state struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
	Base    int64  `json:"baseVersion"`
}

func (c *client) draft(ctx context.Context, path string) (state, error) {
	status, b, e := c.request(ctx, "POST", path, url.Values{"csrf_token": {c.csrf}})
	if e != nil || status != 200 {
		return state{}, fmt.Errorf("draft %d %v", status, e)
	}
	m := stateRE.FindSubmatch(b)
	if len(m) != 2 {
		return state{}, errors.New("missing editor state")
	}
	var s state
	e = json.Unmarshal(m[1], &s)
	return s, e
}
func largePNG() []byte {
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4000, 4000)))
	base := b.Bytes()
	payload := make([]byte, (10<<20)-len(base)-12)
	var chunk bytes.Buffer
	binary.Write(&chunk, binary.BigEndian, uint32(len(payload)))
	data := append([]byte("kbAa"), payload...)
	chunk.Write(data)
	binary.Write(&chunk, binary.BigEndian, crc32.ChecksumIEEE(data))
	return append(append(append([]byte{}, base[:len(base)-12]...), chunk.Bytes()...), base[len(base)-12:]...)
}
func (c *client) upload(ctx context.Context, picture []byte) error {
	s, e := c.draft(ctx, "/drafts")
	if e != nil {
		return e
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, e := mw.CreateFormFile("file", "maximum.png")
	if e != nil {
		return e
	}
	part.Write(picture)
	mw.Close()
	req, e := http.NewRequestWithContext(ctx, "POST", c.origin+"/drafts/"+s.ID+"/media", &body)
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Origin", c.origin)
	req.Header.Set("X-CSRF-Token", c.csrf)
	r, e := c.http.Do(req)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	io.Copy(io.Discard, r.Body)
	if r.StatusCode != 200 {
		return fmt.Errorf("upload status %d", r.StatusCode)
	}
	return nil
}
func exercise(origin, cert, password, hot, category string, duration time.Duration) error {
	reader, e := newClient(origin, cert, "reader", password)
	if e != nil {
		return e
	}
	writers := []*client{}
	for i := 1; i <= 5; i++ {
		c, e := newClient(origin, cert, "writer"+strconv.Itoa(i), password)
		if e != nil {
			return e
		}
		writers = append(writers, c)
	}
	picture := largePNG()
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	start := time.Now()
	var reads, writes, conflicts, failures, uploads atomic.Uint64
	var guard sync.Mutex
	versions := map[int64]int{}
	firstError := ""
	bad := func(err error) {
		if ctx.Err() != nil {
			return
		}
		failures.Add(1)
		guard.Lock()
		if firstError == "" {
			firstError = err.Error()
		}
		guard.Unlock()
		if failures.Load() > 100 {
			cancel()
		}
	}
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() {
			for ctx.Err() == nil {
				path := "/articles/" + hot
				if i%10 == 0 {
					path = "/search?q=" + url.QueryEscape("правила")
				}
				status, b, e := reader.request(ctx, "GET", path, nil)
				if e != nil || status != 200 {
					bad(fmt.Errorf("read status %d: %v", status, e))
				} else if !bytes.Contains(b, []byte("article-body")) && !bytes.Contains(b, []byte("search-results")) {
					bad(errors.New("read did not return content"))
				} else {
					reads.Add(1)
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(30 * time.Millisecond):
				}
			}
		})
	}
	for i, c := range writers {
		wg.Go(func() {
			for n := 0; ctx.Err() == nil; n++ {
				func() {
					s, e := c.draft(ctx, "/articles/"+hot+"/drafts")
					if e != nil {
						bad(e)
						return
					}
					doc := map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": fmt.Sprintf("Правила: редактор %d, правка %d", i, n)}}}}}
					status, b, e := c.request(ctx, "PUT", "/drafts/"+s.ID, map[string]any{"title": "Конкурентные правила", "categoryId": category, "expectedVersion": s.Version, "document": doc})
					if e != nil || status != 200 {
						bad(fmt.Errorf("save status %d %v", status, e))
						return
					}
					var saved struct {
						Version int64 `json:"version"`
					}
					if json.Unmarshal(b, &saved) != nil {
						bad(errors.New("invalid save response"))
						return
					}
					status, b, e = c.request(ctx, "POST", "/drafts/"+s.ID+"/publish", map[string]any{"expectedDraftVersion": saved.Version, "expectedArticleVersion": s.Base})
					if e != nil {
						bad(e)
						return
					}
					if status == 409 {
						conflicts.Add(1)
						return
					}
					if status != 200 {
						bad(fmt.Errorf("publish status %d", status))
						return
					}
					var published struct {
						Version int64 `json:"version"`
					}
					if json.Unmarshal(b, &published) != nil || published.Version != s.Base+1 {
						bad(errors.New("invalid publication version"))
						return
					}
					guard.Lock()
					versions[s.Base]++
					duplicate := versions[s.Base] > 1
					guard.Unlock()
					if duplicate {
						bad(errors.New("multiple winners for one base version"))
					}
					writes.Add(1)
				}()
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
				}
			}
		})
	}
	for range 2 {
		wg.Go(func() {
			if e := reader.upload(ctx, picture); e != nil {
				bad(e)
			} else {
				uploads.Add(1)
			}
		})
	}
	wg.Wait()
	report := map[string]any{"durationSeconds": time.Since(start).Seconds(), "readers": 50, "authors": 5, "articles": 5000, "reads": reads.Load(), "publications": writes.Load(), "conflicts": conflicts.Load(), "uploads": uploads.Load(), "uploadBytesEach": len(picture), "uploadPixelsEach": 16000000, "failures": failures.Load(), "firstError": firstError}
	json.NewEncoder(os.Stdout).Encode(report)
	if failures.Load() != 0 || reads.Load() == 0 || writes.Load() == 0 || uploads.Load() != 2 || time.Since(start) < duration {
		return errors.New("capacity workflow failed")
	}
	return nil
}
