package content

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
)

// AgentDocument accepts the editable text/table subset of the site's native
// document format. Media remains available through the browser editor only.
func AgentDocument(doc json.RawMessage) (json.RawMessage, error) {
	if len(doc) == 0 || len(doc) > 256<<10 || !utf8.Valid(doc) {
		return nil, domain.ErrInvalid
	}
	rendered, err := ValidateAndRender(doc)
	if err != nil || strings.TrimSpace(rendered.PlainText) == "" {
		return nil, domain.ErrInvalid
	}
	var root node
	if json.Unmarshal(doc, &root) != nil || !agentNode(root, "", false) {
		return nil, domain.ErrInvalid
	}
	return json.Marshal(root)
}

func agentNode(n node, parent string, inTable bool) bool {
	switch n.Type {
	case "doc", "blockquote", "tableCell", "tableHeader":
		if len(n.Content) == 0 {
			return false
		}
	case "paragraph", "heading", "hardBreak", "horizontalRule", "codeBlock":
	case "text":
		if n.Text == "" || (parent == "codeBlock" && len(n.Marks) != 0) {
			return false
		}
		seen := make(map[string]bool, len(n.Marks))
		for _, mark := range n.Marks {
			if seen[mark.Type] || (mark.Type == "code" && len(n.Marks) != 1) {
				return false
			}
			seen[mark.Type] = true
		}
	case "bulletList", "orderedList":
		if len(n.Content) == 0 {
			return false
		}
	case "listItem":
		if len(n.Content) == 0 || n.Content[0].Type != "paragraph" {
			return false
		}
	case "table":
		if inTable || len(n.Content) == 0 || len(n.Content[0].Content) == 0 {
			return false
		}
		columns := len(n.Content[0].Content)
		for _, row := range n.Content {
			if len(row.Content) != columns {
				return false
			}
			for _, cell := range row.Content {
				col, err := number(cell.Attrs, "colspan", 1, 1, 1)
				if err != nil || col != 1 {
					return false
				}
				span, err := number(cell.Attrs, "rowspan", 1, 1, 1)
				if err != nil || span != 1 {
					return false
				}
			}
		}
		inTable = true
	case "tableRow":
	default:
		return false
	}
	for _, child := range n.Content {
		if !agentNode(child, n.Type, inTable) {
			return false
		}
	}
	return true
}
