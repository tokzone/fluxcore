package message

import "testing"

func TestContentIsText(t *testing.T) {
	t.Parallel()
	textContent := TextContent("hello")
	if !textContent.IsText() {
		t.Error("TextContent should return true for IsText()")
	}

	imageContent := ImageContent("http://example.com/img.png", "image/png", "")
	if imageContent.IsText() {
		t.Error("ImageContent should return false for IsText()")
	}

	audioContent := AudioContent("http://example.com/audio.mp3", "audio/mp3", "")
	if audioContent.IsText() {
		t.Error("AudioContent should return false for IsText()")
	}
}

func TestContentIsMedia(t *testing.T) {
	t.Parallel()
	textContent := TextContent("hello")
	if textContent.IsMedia() {
		t.Error("TextContent should return false for IsMedia()")
	}

	imageContent := ImageContent("http://example.com/img.png", "image/png", "")
	if !imageContent.IsMedia() {
		t.Error("ImageContent should return true for IsMedia()")
	}

	audioContent := AudioContent("http://example.com/audio.mp3", "audio/mp3", "")
	if !audioContent.IsMedia() {
		t.Error("AudioContent should return true for IsMedia()")
	}
}

func TestExtractAllText(t *testing.T) {
	t.Parallel()
	contents := []Content{
		TextContent("Hello"),
		TextContent(" "),
		TextContent("World"),
		ImageContent("http://example.com/img.png", "image/png", ""),
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

func TestForEachText(t *testing.T) {
	t.Parallel()
	contents := []Content{
		TextContent("Hello"),
		TextContent("World"),
		ImageContent("http://example.com/img.png", "image/png", ""),
	}

	var collected []string
	ForEachText(contents, func(text string) {
		collected = append(collected, text)
	})

	if len(collected) != 2 {
		t.Errorf("expected 2 texts, got %d", len(collected))
	}
	if collected[0] != "Hello" || collected[1] != "World" {
		t.Errorf("expected [Hello, World], got %v", collected)
	}
}

func TestForEachTextEmpty(t *testing.T) {
	t.Parallel()
	called := false
	ForEachText([]Content{}, func(text string) {
		called = true
	})
	if called {
		t.Error("callback should not be called for empty slice")
	}
}