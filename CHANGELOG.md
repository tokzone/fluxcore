# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [0.5.0] - 2024-04-24

### Changed
- Removed EstimateTokens function (token estimation is application-layer concern)
- Made RetryConfig internal: retryConfig, setRetryConfig, getRetryConfig
- Removed unused ImageContent, AudioContent, Clone, WithModel, WithMaxTokens
- Made circuit breaker config internal (circuitBreakerConfig, newEndpointWithConfig)
- Made SSE internal types private: sseEvent, sseParseResult, chunkParser, registerChunkParser
- Simplified routing: one smart algorithm replaces 3 strategies
- Merged ImageData/AudioData into MediaData with type aliases
- Removed deprecated HasUsage field (use IsAccurate)
- Removed deprecated ShouldSkip method (use IsCircuitBreakerOpen)
- Removed deprecated AsImage/AsAudio methods (use AsMedia)

### Added
- Stability tests for circuit breaker recovery, EWMA latency, network resilience
- IsText(), ExtractAllText() helper functions
- Package-level doc.go for all packages
- Key nil check in NewEndpoint constructors
- 90%+ test coverage (Routing 94.2%, Call 90.8%)

### Fixed
- Validate() now only requires Model for Gemini protocol
- doc.go examples updated to match actual API

## [Unreleased]

### Added
- Initial release