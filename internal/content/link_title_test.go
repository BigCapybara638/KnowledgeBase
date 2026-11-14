package content

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEditorLinkTitleRoundTrip(t *testing.T) {
	for _, title := range []string{`null`, `"Подсказка <b> & \"цитата\""`} {
		doc := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Справка","marks":[{"type":"link","attrs":{"href":"https://example.com/help","target":"_blank","rel":"noopener noreferrer nofollow","class":null,"title":` + title + `}}]}]}]}`)
		validated, err := AgentDocument(doc)
		if err != nil {
			t.Fatalf("native editor link rejected: %v", err)
		}
		rendered, err := ValidateAndRender(validated)
		if err != nil {
			t.Fatal(err)
		}
		if title == "null" {
			if strings.Contains(string(rendered.HTML), "title=") {
				t.Fatal("null title rendered")
			}
		} else if !strings.Contains(string(rendered.HTML), `title="Подсказка &lt;b&gt; &amp; &#34;цитата&#34;"`) {
			t.Fatalf("unsafe/lost link title: %s", rendered.HTML)
		}
	}
	for _, title := range []string{`42`, `{}`, `"` + strings.Repeat("x", 4097) + `"`} {
		doc := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Link","marks":[{"type":"link","attrs":{"href":"https://example.com/help","title":` + title + `}}]}]}]}`)
		if _, err := AgentDocument(doc); err == nil {
			t.Fatal("invalid title accepted")
		}
	}
}
