# Contributing

## Development

```bash
# Run tests
go test ./...

# Run with race detection
go test -race ./...

# Check coverage
go test -cover ./...

# Lint
go vet ./...
```

## Code Style

- Run `gofmt -s -w .` before committing
- Follow [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)

## Commit Format

```
type: description

# Types: feat, fix, refactor, docs, test, chore
# Example: feat: add rate limiting support
```

## Pull Request

1. Fork the repository
2. Create a feature branch
3. Make changes and add tests
4. Run `go test ./...`
5. Submit PR