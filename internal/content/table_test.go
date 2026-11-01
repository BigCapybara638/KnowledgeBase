package content

import (
	"strings"
	"testing"
)

func TestTablesRenderAndIndexCellText(t *testing.T) {
	d := []byte(`{"type":"doc","content":[{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableCell","attrs":{"colspan":1,"rowspan":1,"colwidth":null},"content":[{"type":"paragraph","content":[{"type":"text","text":"Ячейка <тест>"}]}]}]}]}]}`)
	r, e := ValidateAndRender(d)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(r.HTML), "<table>") || !strings.Contains(string(r.HTML), "&lt;тест&gt;") || !strings.Contains(r.PlainText, "Ячейка") {
		t.Fatal(r)
	}
}
