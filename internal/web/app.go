package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/BigCapybara638/KnowledgeBase/internal/articles"
	"github.com/BigCapybara638/KnowledgeBase/internal/auth"
	"github.com/BigCapybara638/KnowledgeBase/internal/categories"
	"github.com/BigCapybara638/KnowledgeBase/internal/config"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/media"
	"github.com/BigCapybara638/KnowledgeBase/internal/search"
	"github.com/BigCapybara638/KnowledgeBase/internal/storage"
	"github.com/BigCapybara638/KnowledgeBase/internal/ui"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
)

type userKey struct{}
type App struct {
	search     *search.Service
	media      *media.Service
	articles   *articles.Service
	categories *categories.Service
	cfg        config.Config
	store      *storage.Store
	accounts   *auth.Service
	router     chi.Router
	handler    http.Handler
	views      *template.Template
}

func New(cfg config.Config, s *storage.Store, accounts *auth.Service) (*App, error) {
	views, e := ui.Templates()
	if e != nil {
		return nil, e
	}
	a := &App{cfg: cfg, store: s, accounts: accounts, views: views, router: chi.NewRouter()}
	a.search = search.New(s)
	a.media = media.New(s)
	a.articles = articles.New(s)
	a.categories = categories.New(s)
	r := a.router
	r.Get("/login", a.loginPage)
	r.Post("/login", a.login)
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServerFS(ui.Static())))
	r.Group(func(p chi.Router) {
		p.Use(a.requireSession)
		p.Get("/password", a.passwordPage)
		p.Post("/password", a.password)
		p.Post("/logout", a.logout)
	})
	r.Group(func(p chi.Router) {
		p.Use(a.requireContent)
		p.Get("/", a.home)
		p.Get("/search", a.searchPage)
		p.Get("/articles/{id}", a.article)
		p.Get("/drafts", a.draftsPage)
		p.Post("/drafts", a.createDraft)
		p.Post("/articles/{id}/drafts", a.createDraft)
		p.Get("/drafts/{id}/edit", a.editor)
		p.Put("/drafts/{id}", a.saveDraft)
		p.Post("/drafts/{id}/media", a.uploadImage)
		p.Get("/media/{id}", a.getImage)
		p.Post("/drafts/{id}/publish", a.publishDraft)
		p.Post("/articles/{id}/archive", a.archiveArticle)
		p.Get("/admin/archive", a.archivePage)
		p.Get("/categories/{id}/articles", a.categoryMenu)
		p.Get("/admin/categories", a.categoriesPage)
		p.Post("/admin/categories", a.saveCategory)
		p.Post("/admin/categories/{id}", a.saveCategory)
		p.Post("/admin/categories/{id}/delete", a.deleteCategory)
		p.Get("/admin/users", a.usersPage)
		p.Post("/admin/users", a.createUser)
		p.Post("/admin/users/{id}/reset-password", a.resetPassword)
		p.Post("/admin/users/{id}/disable", a.disableUser)
	})
	protected := csrf.Protect(accounts.CSRFKey, csrf.Secure(!cfg.Dev), csrf.Path("/"), csrf.SameSite(csrf.SameSiteLaxMode), csrf.FieldName("csrf_token"), csrf.MaxAge(43200), csrf.ErrorHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Недопустимый запрос. Обновите страницу.", 403)
	})))(r)
	a.handler = a.middleware(protected)
	return a, nil
}
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/readyz" || r.URL.Path == "/healthz" {
		HealthRoutes(a.store).ServeHTTP(w, r)
		return
	}
	a.handler.ServeHTTP(w, r)
}
func currentUser(r *http.Request) domain.User {
	u, _ := r.Context().Value(userKey{}).(domain.User)
	return u
}
func (a *App) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' blob:; media-src 'self' https: http:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
			if origin := r.Header.Get("Origin"); origin != "" && origin != a.cfg.PublicURL {
				http.Error(w, "Недопустимый источник запроса", 403)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 12<<20)
		}
		if a.cfg.Dev {
			r = csrf.PlaintextHTTPRequest(r)
		}
		// Public links and Origin checks use configured URL, never client-supplied proxy headers.
		for _, h := range []string{"Forwarded", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Forwarded-For"} {
			r.Header.Del(h)
		}
		if c, e := r.Cookie("kb_session"); e == nil {
			if u, e := a.accounts.ResolveSession(r.Context(), c.Value); e == nil {
				r = r.WithContext(context.WithValue(r.Context(), userKey{}, u))
			}
		}
		next.ServeHTTP(w, r)
	})
}
func (a *App) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r).ID == "" {
			if r.Method == "GET" {
				target := "/login?return_to=" + url.QueryEscape(safeReturnTo(r.URL.RequestURI()))
				w.Header().Set("HX-Redirect", target)
				http.Redirect(w, r, target, 303)
			} else {
				http.Error(w, "Войдите в систему", 401)
			}
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (a *App) requireContent(next http.Handler) http.Handler {
	return a.requireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r).MustChangePassword {
			w.Header().Set("HX-Redirect", "/password")
			http.Redirect(w, r, "/password", 303)
			return
		}
		next.ServeHTTP(w, r)
	}))
}
func (a *App) view(w http.ResponseWriter, r *http.Request, status int, page string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	data["User"] = currentUser(r)
	data["CSRFToken"] = csrf.Token(r)
	var b bytes.Buffer
	if e := a.views.ExecuteTemplate(&b, page, data); e != nil {
		log.Printf("template %s: %v", page, e)
		http.Error(w, "Ошибка отображения страницы", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	w.Write(b.Bytes())
}
func (a *App) fail(w http.ResponseWriter, r *http.Request, e error) {
	status := 500
	msg := "Не удалось выполнить операцию"
	switch {
	case errors.Is(e, domain.ErrConflict):
		status = 409
		msg = "Данные уже изменены. Обновите страницу; ваши правки не были перезаписаны."
	case errors.Is(e, domain.ErrForbidden):
		status = 403
		msg = "Нет доступа"
	case errors.Is(e, domain.ErrNotFound):
		status = 404
		msg = "Не найдено"
	case errors.Is(e, domain.ErrInvalid):
		status = 400
		msg = "Проверьте введённые данные"
	case errors.Is(e, domain.ErrBusy):
		status = 503
		msg = "Сервис занят. Повторите попытку."
		w.Header().Set("Retry-After", "1")
	}
	if status == 500 {
		log.Printf("request failed: %v", e)
	}
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]any{"error": msg})
	} else {
		http.Error(w, msg, status)
	}
}
func safeReturnTo(raw string) string {
	decoded, e := url.PathUnescape(raw)
	if e != nil || !strings.HasPrefix(decoded, "/") || strings.HasPrefix(decoded, "//") || strings.ContainsAny(decoded, "\\\r\n") {
		return "/"
	}
	u, e := url.ParseRequestURI(raw)
	if e != nil || u.IsAbs() || u.Host != "" {
		return "/"
	}
	return raw
}
