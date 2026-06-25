BINARY := gh-act
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X=main.version=$(VERSION)

.PHONY: all
all: fmt lint test build

.PHONY: build
build:
	go build -ldflags="$(LDFLAGS)" -o $(BINARY) .

.PHONY: install
install:
	gh extension install .

.PHONY: test
test:
	go test ./...

.PHONY: cover
cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: fmt
fmt:
	go fmt ./...
	gofumpt -w . 2>/dev/null || true

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: vet
vet:
	go vet ./...

.PHONY: clean
clean:
	rm -f $(BINARY) coverage.out
