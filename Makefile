.PHONY: all build test lint coverage clean install fmt vet

BINARY := codefuse
CMD := ./cmd/codefuse

all: fmt vet lint test build

build:
	go build -o $(BINARY) $(CMD)

test:
	go test -race ./...

test-coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --timeout=5m; \
	else \
		echo "golangci-lint not installed. Run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -f $(BINARY) coverage.out coverage.html

install:
	go install $(CMD)

bench:
	go test -bench=. -benchmem ./...
