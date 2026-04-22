package message

import (
	"testing"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected int // approximate range
	}{
		{
			name:     "empty",
			content:  "",
			expected: 0,
		},
		{
			name:     "single_char",
			content:  "a",
			expected: 1,
		},
		{
			name:     "english",
			content:  "Hello world this is a test",
			expected: 6, // ~4 chars per token
		},
		{
			name:     "chinese",
			content:  "你好世界这是一个测试",
			expected: 14, // ~1.5 chars per token
		},
		{
			name:     "mixed",
			content:  "Hello 你好 world 世界",
			expected: 10, // mixed content
		},
		{
			name:     "long_english",
			content:  "The quick brown fox jumps over the lazy dog multiple times",
			expected: 12,
		},
		{
			name:     "long_chinese",
			content:  "人工智能正在改变我们的生活方式和工作方式",
			expected: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EstimateTokens(tt.content)
			// Allow ±50% tolerance
			min := tt.expected * 50 / 100
			max := tt.expected * 150 / 100
			if tt.expected == 0 {
				min = 0
				max = 0
			}
			if result < min || result > max {
				t.Errorf("expected ~%d tokens (range %d-%d), got %d", tt.expected, min, max, result)
			}
		})
	}
}

func TestEstimateTokensFromMessages(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		messages := []Message{
			{Role: "user", Content: []Content{TextContent("Hello world")}},
			{Role: "assistant", Content: []Content{TextContent("Hi there")}},
		}

		result := EstimateTokensFromMessages(messages)
		if result < 5 || result > 25 {
			t.Errorf("expected ~10-15 tokens (range 5-25), got %d", result)
		}
	})

	t.Run("empty_messages", func(t *testing.T) {
		result := EstimateTokensFromMessages([]Message{})
		if result != 0 {
			t.Errorf("expected 0 tokens for empty messages, got %d", result)
		}
	})

	t.Run("single_message", func(t *testing.T) {
		messages := []Message{
			{Role: "user", Content: []Content{TextContent("Hello")}},
		}
		result := EstimateTokensFromMessages(messages)
		if result < 1 {
			t.Errorf("expected at least 1 token, got %d", result)
		}
	})

	t.Run("non_text_content_skipped", func(t *testing.T) {
		messages := []Message{
			{Role: "user", Content: []Content{ImageContent("http://example.com/img.png", "image/png", "")}},
		}
		result := EstimateTokensFromMessages(messages)
		// Should count role token only
		if result < 1 {
			t.Errorf("expected at least 1 token for role, got %d", result)
		}
	})
}

func TestEstimateTokensChineseBoundaries(t *testing.T) {
	t.Run("all_chinese", func(t *testing.T) {
		// String with all Chinese characters
		content := "人工智能机器学习深度学习"
		result := EstimateTokens(content)
		// Each Chinese char ~1.5 tokens, so 10 chars ≈ 7-15 tokens
		if result < 5 || result > 20 {
			t.Errorf("expected ~10 tokens for all Chinese, got %d", result)
		}
	})

	t.Run("all_english", func(t *testing.T) {
		// String with all English characters
		content := "artificial intelligence machine learning"
		result := EstimateTokens(content)
		// ~4 chars per token
		if result < 5 || result > 20 {
			t.Errorf("expected ~10 tokens for all English, got %d", result)
		}
	})

	t.Run("mixed_heavy_chinese", func(t *testing.T) {
		// Mostly Chinese with some English
		content := "使用GPT模型进行文本生成"
		result := EstimateTokens(content)
		if result < 5 {
			t.Errorf("expected at least 5 tokens, got %d", result)
		}
	})
}