package content

import (
	"bytes"
	"encoding/json"
	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
	"github.com/google/uuid"
	"html"
	"html/template"
	"io"
	"net/url"
	"strconv"
	"strings"
)

type Rendered struct {
	HTML      template.HTML
	PlainText string
	MediaIDs  []domain.ID
}
type node struct {
	Type    string                     `json:"type"`
	Text    string                     `json:"text,omitempty"`
	Attrs   map[string]json.RawMessage `json:"attrs,omitempty"`
	Content []node                     `json:"content,omitempty"`
	Marks   []mark                     `json:"marks,omitempty"`
}
type mark struct {
	Type  string                     `json:"type"`
	Attrs map[string]json.RawMessage `json:"attrs,omitempty"`
}
type renderer struct {
	html, plain strings.Builder
	count       int
	media       []domain.ID
}

func ValidateAndRender(doc json.RawMessage) (Rendered, error) {
	if len(doc) == 0 || len(doc) > 2<<20 {
		return Rendered{}, domain.ErrInvalid
	}
	var n node
	d := json.NewDecoder(bytes.NewReader(doc))
	d.DisallowUnknownFields()
	if d.Decode(&n) != nil {
		return Rendered{}, domain.ErrInvalid
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return Rendered{}, domain.ErrInvalid
	}
	if n.Type != "doc" {
		return Rendered{}, domain.ErrInvalid
	}
	r := &renderer{}
	if e := r.render(n, "", 0); e != nil {
		return Rendered{}, e
	}
	return Rendered{HTML: template.HTML(r.html.String()), PlainText: strings.TrimSpace(r.plain.String()), MediaIDs: r.media}, nil
}
func attrsOnly(m map[string]json.RawMessage, keys ...string) bool {
	for k := range m {
		ok := false
		for _, s := range keys {
			if s == k {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	return true
}
func number(m map[string]json.RawMessage, k string, def, min, max int) (int, error) {
	b := m[k]
	if len(b) == 0 || string(b) == "null" {
		return def, nil
	}
	var n int
	if json.Unmarshal(b, &n) != nil || n < min || n > max {
		return 0, domain.ErrInvalid
	}
	return n, nil
}
func str(m map[string]json.RawMessage, k string) (string, error) {
	b := m[k]
	if len(b) == 0 || string(b) == "null" {
		return "", nil
	}
	var s string
	if json.Unmarshal(b, &s) != nil || len(s) > 4096 {
		return "", domain.ErrInvalid
	}
	return s, nil
}
func safeLink(raw string) bool {
	u, e := url.Parse(raw)
	if e != nil || strings.ContainsAny(raw, "\r\n\\") {
		return false
	}
	if (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil {
		return true
	}
	if u.Scheme == "" && u.Host == "" && strings.HasPrefix(u.Path, "/articles/") {
		_, e = uuid.Parse(strings.TrimPrefix(u.Path, "/articles/"))
		return e == nil
	}
	return false
}
func validChild(parent, child string) bool {
	switch parent {
	case "":
		return child == "doc"
	case "doc", "blockquote", "listItem":
		return child == "paragraph" || child == "heading" || child == "blockquote" || child == "bulletList" || child == "orderedList" || child == "codeBlock" || child == "horizontalRule"
	case "bulletList", "orderedList":
		return child == "listItem"
	case "paragraph", "heading":
		return child == "text" || child == "hardBreak"
	case "codeBlock":
		return child == "text"
	}
	return false
}
func (r *renderer) render(n node, parent string, depth int) error {
	r.count++
	if depth > 32 || r.count > 10000 || !validChild(parent, n.Type) {
		return domain.ErrInvalid
	}
	if n.Type != "text" && (n.Text != "" || len(n.Marks) != 0) {
		return domain.ErrInvalid
	}
	if n.Type == "text" {
		if len(n.Attrs) != 0 || len(n.Content) != 0 || len(n.Marks) > 16 {
			return domain.ErrInvalid
		}
		text := html.EscapeString(n.Text)
		for i := len(n.Marks) - 1; i >= 0; i-- {
			m := n.Marks[i]
			tag := ""
			switch m.Type {
			case "bold":
				tag = "strong"
			case "italic":
				tag = "em"
			case "underline":
				tag = "u"
			case "strike":
				tag = "s"
			case "code":
				tag = "code"
			case "link":
				if !attrsOnly(m.Attrs, "href", "target", "rel", "class") {
					return domain.ErrInvalid
				}
				href, e := str(m.Attrs, "href")
				if e != nil || !safeLink(href) {
					return domain.ErrInvalid
				}
				text = `<a href="` + html.EscapeString(href) + `" rel="noopener noreferrer">` + text + `</a>`
				continue
			default:
				return domain.ErrInvalid
			}
			if len(m.Attrs) != 0 {
				return domain.ErrInvalid
			}
			text = "<" + tag + ">" + text + "</" + tag + ">"
		}
		r.html.WriteString(text)
		r.plain.WriteString(n.Text)
		return nil
	}
	tag, extra := "", ""
	switch n.Type {
	case "doc":
	case "paragraph":
		tag = "p"
	case "heading":
		if !attrsOnly(n.Attrs, "level") {
			return domain.ErrInvalid
		}
		level, e := number(n.Attrs, "level", 1, 1, 3)
		if e != nil {
			return e
		}
		tag = "h" + strconv.Itoa(level)
	case "blockquote":
		tag = "blockquote"
	case "bulletList":
		tag = "ul"
	case "orderedList":
		if !attrsOnly(n.Attrs, "start", "type") {
			return domain.ErrInvalid
		}
		v, e := number(n.Attrs, "start", 1, 1, 100000)
		if e != nil {
			return e
		}
		tag = "ol"
		extra = ` start="` + strconv.Itoa(v) + `"`
	case "listItem":
		tag = "li"
	case "codeBlock":
		if !attrsOnly(n.Attrs, "language") {
			return domain.ErrInvalid
		}
		tag = "pre"
	case "hardBreak":
		if len(n.Content) > 0 || len(n.Attrs) > 0 {
			return domain.ErrInvalid
		}
		r.html.WriteString("<br>")
		r.plain.WriteByte('\n')
		return nil
	case "horizontalRule":
		if len(n.Content) > 0 || len(n.Attrs) > 0 {
			return domain.ErrInvalid
		}
		r.html.WriteString("<hr>")
		r.plain.WriteByte('\n')
		return nil
	default:
		return domain.ErrInvalid
	}
	if n.Type != "heading" && n.Type != "orderedList" && n.Type != "codeBlock" && len(n.Attrs) != 0 {
		return domain.ErrInvalid
	}
	if parent == "codeBlock" && len(n.Marks) > 0 {
		return domain.ErrInvalid
	}
	if tag != "" {
		r.html.WriteString("<" + tag + extra + ">")
	}
	for _, c := range n.Content {
		if e := r.render(c, n.Type, depth+1); e != nil {
			return e
		}
	}
	if tag != "" {
		r.html.WriteString("</" + tag + ">")
		r.plain.WriteByte('\n')
	}
	return nil
}
