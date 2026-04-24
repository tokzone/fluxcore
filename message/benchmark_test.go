package message

import (
	"strings"
	"testing"
)

func BenchmarkEstimateTokens(b *testing.B) {
	english := "The quick brown fox jumps over the lazy dog"
	chinese := "人工智能正在改变我们的生活方式和工作方式"
	mixed := "Hello 你好 world 世界 this 这是一个 test 测试"

	b.Run("english", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			EstimateTokens(english)
		}
	})

	b.Run("chinese", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			EstimateTokens(chinese)
		}
	})

	b.Run("mixed", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			EstimateTokens(mixed)
		}
	})

	b.Run("long_english", func(b *testing.B) {
		long := strings.Repeat(english, 100)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			EstimateTokens(long)
		}
	})

	b.Run("long_chinese", func(b *testing.B) {
		long := strings.Repeat(chinese, 100)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			EstimateTokens(long)
		}
	})
}

