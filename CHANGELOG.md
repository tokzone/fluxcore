# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [Unreleased]

### Added
- Initial release

### Changed
- Simplified routing: one smart algorithm replaces 3 strategies
- Merged ImageData/AudioData into MediaData with type aliases
- Removed deprecated HasUsage field (use IsAccurate)
- Removed deprecated ShouldSkip method (use IsCircuitBreakerOpen)
- Removed deprecated AsImage/AsAudio methods (use AsMedia)
- Added IsText(), IsMedia(), ExtractAllText() helper functions
- Added configurable HTTP timeout via SetConfig()
- Added package-level doc.go for all packages
- Added Key nil check in NewEndpoint constructors

### Fixed
- Validate() now only requires Model for Gemini protocol