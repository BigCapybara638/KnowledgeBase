package content

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
)

func TestAgentDocumentRejectsUnsupportedOrUneditableContent(t *testing.T) {
	wrap := func(s string) string { return `{"type":"doc","content":[` + s + `]}` }
	paragraph := `{"type":"paragraph","content":[{"type":"text","text":"Text"}]}`
	cell := `{"type":"tableCell","content":[` + paragraph + `]}`
	row := `{"type":"tableRow","content":[` + cell + `]}`
	for name, raw := range map[string]string{
		"empty":                      `{"type":"doc","content":[]}`,
		"blank":                      wrap(`{"type":"paragraph"}`),
		"video":                      wrap(`{"type":"video","attrs":{"src":"https://example.com/a.mp4"}}`),
		"image":                      wrap(`{"type":"image","attrs":{"src":"/media/00000000-0000-0000-0000-000000000001"}}`),
		"html":                       wrap(`{"type":"html","text":"<b>Hi</b>"}`),
		"unknown field":              `{"type":"doc","unsafe":true,"content":[` + paragraph + `]}`,
		"heading":                    wrap(`{"type":"heading","attrs":{"level":4},"content":[{"type":"text","text":"Text"}]}`),
		"unsafe link":                wrap(`{"type":"paragraph","content":[{"type":"text","text":"Text","marks":[{"type":"link","attrs":{"href":"javascript:alert(1)"}}]}]}`),
		"unsafe color":               wrap(`{"type":"paragraph","content":[{"type":"text","text":"Text","marks":[{"type":"textStyle","attrs":{"color":"red;position:fixed"}}]}]}`),
		"empty text":                 wrap(`{"type":"paragraph","content":[{"type":"text","text":""}]}`),
		"duplicate marks":            wrap(`{"type":"paragraph","content":[{"type":"text","text":"Text","marks":[{"type":"bold"},{"type":"bold"}]}]}`),
		"inline code excludes marks": wrap(`{"type":"paragraph","content":[{"type":"text","text":"Text","marks":[{"type":"bold"},{"type":"code"}]}]}`),
		"list item start":            wrap(`{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"heading","content":[{"type":"text","text":"Text"}]}]}]}`),
		"empty list":                 wrap(`{"type":"bulletList"}`),
		"empty table":                wrap(`{"type":"table"}`),
		"ragged table":               wrap(`{"type":"table","content":[` + row + `,{"type":"tableRow","content":[` + cell + `,` + cell + `]}]}`),
		"merged cells":               wrap(`{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableCell","attrs":{"colspan":2},"content":[` + paragraph + `]}]}]}`),
		"nested table":               wrap(`{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableCell","content":[{"type":"table","content":[` + row + `]}]}]}]}`),
		"styled code":                wrap(`{"type":"codeBlock","content":[{"type":"text","text":"code","marks":[{"type":"bold"}]}]}`),
		"oversized":                  wrap(`{"type":"paragraph","content":[{"type":"text","text":"` + strings.Repeat("x", 256<<10) + `"}]}`),
		"too deep":                   wrap(strings.Repeat(`{"type":"blockquote","content":[`, 33) + paragraph + strings.Repeat(`]}`, 33)),
		"too many nodes":             wrap(strings.Repeat(`{"type":"paragraph"},`, 10000) + paragraph),
		"invalid UTF8":               wrap(`{"type":"paragraph","content":[{"type":"text","text":"` + string([]byte{0xff}) + `"}]}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := AgentDocument(json.RawMessage(raw)); !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("invalid document accepted: %v", err)
			}
		})
	}
}

func TestAgentDocumentSupportsEditorTextBlocks(t *testing.T) {
	raw := json.RawMessage(`{"type":"doc","content":[{"type":"heading","attrs":{"level":3},"content":[{"type":"text","text":"Инструкция"}]},{"type":"paragraph","content":[{"type":"text","text":"Текст <script>","marks":[{"type":"italic"},{"type":"underline"},{"type":"strike"}]},{"type":"hardBreak"},{"type":"text","text":"Продолжение","marks":[{"type":"code"}]}]},{"type":"orderedList","attrs":{"start":2},"content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Шаг"}]}]}]},{"type":"blockquote","content":[{"type":"paragraph","content":[{"type":"text","text":"Цитата"}]}]},{"type":"codeBlock","attrs":{"language":"go"},"content":[{"type":"text","text":"fmt.Println(1)"}]},{"type":"horizontalRule"}]}`)
	doc, err := AgentDocument(raw)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := ValidateAndRender(doc)
	if err != nil || strings.Contains(string(rendered.HTML), "<script>") {
		t.Fatal("unsafe rendering", err)
	}
	for _, part := range []string{"<h3>", "<em>", "<u>", "<s>", "<code>", "<br>", `<ol start="2">`, "<blockquote>", "<pre>", "<hr>"} {
		if !strings.Contains(string(rendered.HTML), part) {
			t.Fatalf("lost %s", part)
		}
	}
}
