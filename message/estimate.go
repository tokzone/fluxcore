package message

// EstimateTokens estimates token count for a string
// Chinese characters: ~1.5 tokens per character
// English: ~4 characters per token
func EstimateTokens(content string) int {
	if content == "" {
		return 0
	}

	chineseCount := 0
	totalChars := 0
	for _, r := range content {
		totalChars++
		if r >= 0x4e00 && r <= 0x9fff {
			chineseCount++
		}
	}

	// Chinese: ~1.5 tokens per char, Non-Chinese: ~4 chars per token
	return chineseCount*2/3 + (totalChars-chineseCount)/4 + 1
}

// EstimateTokensFromMessages estimates total input tokens from messages
func EstimateTokensFromMessages(messages []Message) int {
	total := 0
	for _, msg := range messages {
		// Role token
		total += 1
		// Content tokens
		total += EstimateTokens(ExtractAllText(msg.Content))
	}
	return total
}