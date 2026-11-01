package content

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderingEscapesUserText(t *testing.T) {
	d := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"<script>alert(1)</script> & текст","marks":[{"type":"bold"}]}]}]}`)
	r, e := ValidateAndRender(d)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(r.HTML), "<script>") || !strings.Contains(string(r.HTML), "<strong>&lt;script&gt;") || !strings.Contains(r.PlainText, "текст") {
		t.Fatal(r)
	}
}
func TestRejectsScriptURLAndUnknownNodes(t *testing.T) {
	for _, d := range []string{`{"type":"doc","content":[{"type":"script"}]}`, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"javascript:alert(1)"}}]}]}]}`, `{"type":"doc","attrs":{"onclick":"evil()"},"content":[]}`} {
		if _, e := ValidateAndRender([]byte(d)); e == nil {
			t.Fatal("unsafe document accepted", d)
		}
	}
}
func TestDocumentLimits(t *testing.T) {
	large := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + strings.Repeat("a", 2<<20) + `"}]}]}`
	if _, e := ValidateAndRender([]byte(large)); e == nil {
		t.Fatal("oversized document accepted")
	}
	nested := `{"type":"paragraph"}`
	for range 33 {
		nested = `{"type":"blockquote","content":[` + nested + `]}`
	}
	if _, e := ValidateAndRender([]byte(`{"type":"doc","content":[` + nested + `]}`)); e == nil {
		t.Fatal("deep document accepted")
	}
	many := `{"type":"doc","content":[` + strings.Repeat(`{"type":"paragraph"},`, 10000) + `{"type":"paragraph"}]}`
	if _, e := ValidateAndRender([]byte(many)); e == nil {
		t.Fatal("too many nodes accepted")
	}
}
