package content

import (
	"strings"
	"testing"
)

func TestPlainDocumentPreservesTextAndLineBreaks(t *testing.T) {
	for _, text := range []string{"Привет 🚀\nВторая строка\n\nТретий абзац", "<script>alert('x')</script>", strings.Repeat("я", 1000)} {
		doc, err := PlainDocument(text)
		if err != nil {
			t.Fatal(err)
		}
		rendered, err := ValidateAndRender(doc)
		if err != nil || rendered.PlainText != text {
			t.Fatalf("round trip: %q %v", rendered.PlainText, err)
		}
		if strings.Contains(string(rendered.HTML), "<script>") {
			t.Fatal("raw HTML was interpreted")
		}
	}
	doc, err := PlainDocument("Первая\r\n\r\nВторая\rТретья")
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := ValidateAndRender(doc)
	if err != nil || rendered.PlainText != "Первая\n\nВторая\nТретья" {
		t.Fatal("line ending normalization failed")
	}
}

func TestPlainDocumentRejectsInvalidBody(t *testing.T) {
	for _, text := range []string{"", " \n\t", string([]byte{0xff}), strings.Repeat("x", (256<<10)+1), strings.Repeat("x\n", 10001)} {
		if _, err := PlainDocument(text); err == nil {
			t.Fatal("invalid or oversized body accepted")
		}
	}
}
