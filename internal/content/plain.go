package content

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
)

// PlainDocument converts plain text to the editor's validated document format.
func PlainDocument(text string) (json.RawMessage, error) {
	if len(text) > 256<<10 || !utf8.ValidString(text) || strings.TrimSpace(text) == "" {
		return nil, domain.ErrInvalid
	}
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	paragraphs := []node{}
	for _, line := range strings.Split(text, "\n") {
		paragraph := node{Type: "paragraph"}
		if line != "" {
			paragraph.Content = []node{{Type: "text", Text: line}}
		}
		paragraphs = append(paragraphs, paragraph)
	}
	doc, err := json.Marshal(node{Type: "doc", Content: paragraphs})
	if err != nil {
		return nil, err
	}
	if _, err = ValidateAndRender(doc); err != nil {
		return nil, err
	}
	return doc, nil
}
