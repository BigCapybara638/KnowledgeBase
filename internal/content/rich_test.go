package content

import (
	"strings"
	"testing"
)

func TestRichFormattingSurvivesRendering(t *testing.T) {
	doc := []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Цвет","marks":[{"type":"textStyle","attrs":{"color":"#123456","backgroundColor":"#ffeedd"}}]}]},{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableCell","attrs":{"colspan":1,"rowspan":1,"colwidth":[180],"height":64,"backgroundColor":"#112233"},"content":[{"type":"paragraph"}]}]}]},{"type":"video","attrs":{"src":"https://example.com/demo.mp4","title":"Обучение"}}]}`)
	r, err := ValidateAndRender(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"color:#123456", "background-color:#ffeedd", "width:180px", "height:64px", "background-color:#112233", `<video controls preload="metadata"`, `src="https://example.com/demo.mp4"`} {
		if !strings.Contains(string(r.HTML), want) {
			t.Fatalf("missing %q in %s", want, r.HTML)
		}
	}
}

func TestRejectsUnsafeRichAttributes(t *testing.T) {
	for _, doc := range []string{
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"X","marks":[{"type":"textStyle","attrs":{"color":"red;position:fixed"}}]}]}]}`,
		`{"type":"doc","content":[{"type":"video","attrs":{"src":"javascript:alert(1)"}}]}`,
		`{"type":"doc","content":[{"type":"video","attrs":{"src":"https://user:pass@example.com/x.mp4"}}]}`,
		`{"type":"doc","content":[{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableCell","attrs":{"colwidth":[-1]},"content":[{"type":"paragraph"}]}]}]}]}`,
		`{"type":"doc","content":[{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableCell","attrs":{"height":999999},"content":[{"type":"paragraph"}]}]}]}]}`,
	} {
		if _, err := ValidateAndRender([]byte(doc)); err == nil {
			t.Fatalf("unsafe document accepted: %s", doc)
		}
	}
}

func TestTableUsesExplicitColumnWidths(t *testing.T) {
	doc := []byte(`{"type":"doc","content":[{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableCell","attrs":{"colwidth":[180]},"content":[{"type":"paragraph"}]},{"type":"tableCell","attrs":{"colwidth":[220]},"content":[{"type":"paragraph"}]}]}]}]}`)
	r, err := ValidateAndRender(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(r.HTML), `<table style="width:400px">`) {
		t.Fatal(r.HTML)
	}
}
