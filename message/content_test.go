package message

import "testing"

func TestContentIsText(t *testing.T) {
	t.Parallel()
	textContent := TextContent("hello")
	if !textContent.IsText() {
		t.Error("TextContent should return true for IsText()")
	}
}


func TestExtractAllText(t *testing.T) {
	t.Parallel()
	contents := []Content{
		TextContent("Hello"),
		TextContent(" "),
		TextContent("World"),
	}

	result := ExtractAllText(contents)
	expected := "Hello World"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestExtractAllTextEmpty(t *testing.T) {
	t.Parallel()
	result := ExtractAllText([]Content{})
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

