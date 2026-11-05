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
	case "doc", "blockquote", "listItem", "tableCell", "tableHeader":
		return child == "video" || child == "image" || child == "table" || child == "paragraph" || child == "heading" || child == "blockquote" || child == "bulletList" || child == "orderedList" || child == "codeBlock" || child == "horizontalRule"
	case "bulletList", "orderedList":
		return child == "listItem"
	case "paragraph", "heading":
		return child == "text" || child == "hardBreak"
	case "table":
		return child == "tableRow"
	case "tableRow":
		return child == "tableCell" || child == "tableHeader"
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
			case "textStyle":
				if !attrsOnly(m.Attrs, "color", "backgroundColor") {
					return domain.ErrInvalid
				}
				style, err := colorStyle(m.Attrs)
				if err != nil {
					return err
				}
				text = `<span style="` + style + `">` + text + `</span>`
				continue
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
	if n.Type == "video" {
		if !attrsOnly(n.Attrs, "src", "title") || len(n.Content) != 0 {
			return domain.ErrInvalid
		}
		src, err := str(n.Attrs, "src")
		if err != nil {
			return err
		}
		if strings.HasPrefix(src, "/media/") {
			id := strings.TrimPrefix(src, "/media/")
			if _, err = uuid.Parse(id); err != nil {
				return domain.ErrInvalid
			}
			r.media = append(r.media, domain.ID(id))
		} else {
			u, err := url.Parse(src)
			if err != nil || !safeLink(src) || (u.Scheme != "https" && u.Scheme != "http") {
				return domain.ErrInvalid
			}
		}
		title, err := str(n.Attrs, "title")
		if err != nil {
			return err
		}
		r.html.WriteString(`<video controls preload="metadata" src="` + html.EscapeString(src) + `" title="` + html.EscapeString(title) + `"><a href="` + html.EscapeString(src) + `" rel="noopener noreferrer">Открыть видео</a></video>`)
		r.plain.WriteString(" " + title + " ")
		return nil
	}
	if n.Type == "image" {
		if !attrsOnly(n.Attrs, "src", "alt", "title", "width", "height") || len(n.Content) != 0 {
			return domain.ErrInvalid
		}
		src, e := str(n.Attrs, "src")
		if e != nil || !strings.HasPrefix(src, "/media/") {
			return domain.ErrInvalid
		}
		id := strings.TrimPrefix(src, "/media/")
		if _, e = uuid.Parse(id); e != nil {
			return domain.ErrInvalid
		}
		alt, e := str(n.Attrs, "alt")
		if e != nil {
			return e
		}
		title, e := str(n.Attrs, "title")
		if e != nil {
			return e
		}
		for _, key := range []string{"width", "height"} {
			if _, e = number(n.Attrs, key, 0, 1, 25000000); e != nil {
				return e
			}
		}
		r.html.WriteString(`<img src="` + html.EscapeString(src) + `" alt="` + html.EscapeString(alt) + `" title="` + html.EscapeString(title) + `" loading="lazy">`)
		r.plain.WriteString(" " + alt + " ")
		r.media = append(r.media, domain.ID(id))
		return nil
	}
	tag, extra := "", ""
	switch n.Type {
	case "doc":
	case "table":
		tag = "table"
		if width := tableWidth(n); width > 0 {
			extra = ` style="width:` + strconv.Itoa(width) + `px"`
		}
	case "tableRow":
		tag = "tr"
	case "tableCell", "tableHeader":
		if !attrsOnly(n.Attrs, "colspan", "rowspan", "colwidth", "align", "height", "backgroundColor") {
			return domain.ErrInvalid
		}
		col, e := number(n.Attrs, "colspan", 1, 1, 100)
		if e != nil {
			return e
		}
		row, e := number(n.Attrs, "rowspan", 1, 1, 100)
		if e != nil {
			return e
		}
		align, e := str(n.Attrs, "align")
		if e != nil || (align != "" && align != "left" && align != "right" && align != "center") {
			return domain.ErrInvalid
		}
		tag = "td"
		if n.Type == "tableHeader" {
			tag = "th"
		}
		extra = ` colspan="` + strconv.Itoa(col) + `" rowspan="` + strconv.Itoa(row) + `"`
		if align != "" {
			extra += ` align="` + align + `"`
		}
		style, err := cellStyle(n.Attrs, col)
		if err != nil {
			return err
		}
		if style != "" {
			extra += ` style="` + style + `"`
		}
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
	if n.Type != "heading" && n.Type != "orderedList" && n.Type != "codeBlock" && n.Type != "tableCell" && n.Type != "tableHeader" && len(n.Attrs) != 0 {
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
