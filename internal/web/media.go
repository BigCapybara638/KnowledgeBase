package web

import (
	"errors"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/BigCapybara638/KnowledgeBase/internal/media"
	"github.com/go-chi/chi/v5"
	"net/http"
	"time"
)

func (a *App) uploadImage(w http.ResponseWriter, r *http.Request) {
	mr, e := r.MultipartReader()
	if e != nil {
		a.fail(w, r, domain.ErrInvalid)
		return
	}
	part, e := mr.NextPart()
	if e != nil || part.FormName() != "file" || part.FileName() == "" {
		a.fail(w, r, domain.ErrInvalid)
		return
	}
	defer part.Close()
	m, e := a.media.Upload(r.Context(), currentUser(r).ID, domain.ID(chi.URLParam(r, "id")), part, part.FileName())
	if errors.Is(e, media.ErrUploadLimit) {
		w.Header().Set("Retry-After", "1")
		http.Error(w, "Слишком много одновременных загрузок", 429)
		return
	}
	if e != nil {
		a.fail(w, r, e)
		return
	}
	jsonReply(w, map[string]any{"id": m.ID, "src": "/media/" + string(m.ID), "width": m.Width, "height": m.Height})
}
func (a *App) getImage(w http.ResponseWriter, r *http.Request) {
	f, e := a.media.Open(r.Context(), currentUser(r).ID, domain.ID(chi.URLParam(r, "id")))
	if e != nil {
		a.fail(w, r, e)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", f.Media.Type)
	w.Header().Set("Content-Disposition", "inline")
	http.ServeContent(w, r, string(f.Media.ID), time.Time{}, f)
}
