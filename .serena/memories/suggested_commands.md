# Suggested Development Commands

## Build
```bash
go build -o plexmusic-tui              # Development build
go build -ldflags="-s -w" -o plexmusic-tui  # Release build
```

## Run
```bash
./plexmusic-tui                        # Run the application
```

## Testing
```bash
go test ./...                          # Run all tests
go test ./internal/...                 # Test specific package
```

## Formatting & Linting
```bash
go fmt ./...                           # Format code
gofmt -w .                             # Alternative format
go vet ./...                           # Vet for issues
golangci-lint run ./...                # Comprehensive linting (if installed)
```

## Module Management
```bash
go mod download                        # Download dependencies
go mod tidy                            # Clean up dependencies
go mod verify                          # Verify dependencies
```

## Cross-Compilation Examples
```bash
GOOS=linux GOARCH=amd64 go build -o plexmusic-tui-linux-amd64
GOOS=darwin GOARCH=arm64 go build -o plexmusic-tui-darwin-arm64
GOOS=windows GOARCH=amd64 go build -o plexmusic-tui.exe
```

## Clean Build
```bash
go clean
rm -f plexmusic-tui
```
