package ui

import (
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"strings"
	"time"
)

//go:embed templates/*.html static
var files embed.FS

func Templates() (*template.Template, error) {
	return template.New("").Funcs(template.FuncMap{"date": func(n int64) string {
		if n == 0 {
			return ""
		}
		return time.Unix(n, 0).Format("02.01.2006")
	}, "snippet": func(s string) string {
		r := []rune(strings.Join(strings.Fields(s), " "))
		if len(r) > 160 {
			return string(r[:160]) + "…"
		}
		return string(r)
	}, "json": func(v any) (template.JS, error) { b, e := json.Marshal(v); return template.JS(b), e }}).ParseFS(files, "templates/*.html")
}
func Static() fs.FS { sub, _ := fs.Sub(files, "static"); return sub }
