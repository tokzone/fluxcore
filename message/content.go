package message

import (
	"encoding/json"
	"strings"
)

// ContentData is a sealed interface for content data types.
// Only TextData and MediaData implement this interface.
type ContentData interface {
	isContentData()
}

// TextData represents text content.
type TextData string

func (TextData) isContentData() {}

// MediaData represents media content (image, audio, video).
type MediaData struct {
	URL       string `json:"url,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	Base64    string `json:"base64,omitempty"`
}

func (MediaData) isContentData() {}

// Content represents multimodal content
type Content struct {
	Type string      `json:"type"` // text, image, audio
	Data ContentData `json:"-"`
}

// MarshalJSON implements custom JSON marshaling for Content.
func (c Content) MarshalJSON() ([]byte, error) {
	type jsonContent struct {
		Type string      `json:"type"`
		Data interface{} `json:"data"`
	}
	jc := jsonContent{Type: c.Type}
	switch d := c.Data.(type) {
	case TextData:
		jc.Data = string(d)
	case MediaData:
		jc.Data = d
	}
	return json.Marshal(jc)
}

// UnmarshalJSON implements custom JSON unmarshaling for Content.
func (c *Content) UnmarshalJSON(data []byte) error {
	type jsonContent struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	var jc jsonContent
	if err := json.Unmarshal(data, &jc); err != nil {
		return err
	}
	c.Type = jc.Type
	switch jc.Type {
	case "text":
		var text string
		if err := json.Unmarshal(jc.Data, &text); err != nil {
			return err
		}
		c.Data = TextData(text)
	case "image", "audio":
		var media MediaData
		if err := json.Unmarshal(jc.Data, &media); err != nil {
			return err
		}
		c.Data = media
	}
	return nil
}

// TextContent creates text content
func TextContent(text string) Content {
	return Content{
		Type: "text",
		Data: TextData(text),
	}
}

// AsText returns the text if this is TextData, otherwise empty string.
func (c Content) AsText() string {
	if td, ok := c.Data.(TextData); ok {
		return string(td)
	}
	return ""
}

// IsText returns true if this is text content.
func (c Content) IsText() bool {
	return c.Type == "text"
}

// ExtractAllText concatenates all text from content items.
func ExtractAllText(contents []Content) string {
	var sb strings.Builder
	for _, c := range contents {
		if c.IsText() {
			sb.WriteString(c.AsText())
		}
	}
	return sb.String()
}

