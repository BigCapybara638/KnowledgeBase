package content

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/BigCapybara638/KnowledgeBase/internal/domain"
)

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// Only normalized literal colors reach HTML; arbitrary CSS is never rendered.
func colorStyle(attrs map[string]json.RawMessage) (string, error) {
	style := ""
	for _, pair := range [][2]string{{"color", "color"}, {"backgroundColor", "background-color"}} {
		value, err := str(attrs, pair[0])
		if err != nil {
			return "", err
		}
		if value == "" {
			continue
		}
		if !hexColor.MatchString(value) {
			return "", domain.ErrInvalid
		}
		style += pair[1] + ":" + strings.ToLower(value) + ";"
	}
	return style, nil
}

func cellStyle(attrs map[string]json.RawMessage, colspan int) (string, error) {
	style, err := colorStyle(attrs)
	if err != nil {
		return "", err
	}
	height, err := number(attrs, "height", 0, 16, 2000)
	if err != nil {
		return "", err
	}
	if height > 0 {
		style += "height:" + strconv.Itoa(height) + "px;"
	}
	if raw := attrs["colwidth"]; len(raw) > 0 && string(raw) != "null" {
		var widths []int
		if json.Unmarshal(raw, &widths) != nil || len(widths) != colspan {
			return "", domain.ErrInvalid
		}
		total := 0
		for _, width := range widths {
			// Tiptap uses zero for an as-yet unmeasured column in a merged cell.
			if width < 0 || width > 2000 {
				return "", domain.ErrInvalid
			}
			total += width
		}
		if total > 0 {
			style += "width:" + strconv.Itoa(total) + "px;"
		}
	}
	return style, nil
}

// A fixed total prevents the global table width from stretching explicitly sized columns.
func tableWidth(n node) int {
	if len(n.Content) == 0 {
		return 0
	}
	total := 0
	for _, cell := range n.Content[0].Content {
		var widths []int
		if json.Unmarshal(cell.Attrs["colwidth"], &widths) != nil || len(widths) == 0 {
			return 0
		}
		for _, width := range widths {
			if width <= 0 || width > 2000 {
				return 0
			}
			total += width
		}
	}
	return total
}
