# TUI Orchestrator (Go + Bubbletea)

set working-directory := "orchestra"

# Run the TUI
run:
    go run .

# Build binary
build:
    mkdir -p ../bin
    go build -o ../bin/orchestra

# Tidy dependencies
tidy:
    go mod tidy

# Format code
fmt:
    go fmt ./...

# Clean
clean:
    rm -f ../bin/orchestra
