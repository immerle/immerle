BINARY      := gossignol
PKG         := ./cmd/gossignol
BIN_DIR     := bin
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X main.version=$(VERSION)

SWAG := go run github.com/swaggo/swag/v2/cmd/swag@v2.0.0-rc5

.PHONY: all build run test test-race lint fmt vet tidy clean docker docker-up docker-down ci openapi openapi-check

all: build

## build: compile the server binary
build:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) $(PKG)

## run: run the server (loads .env if present)
run:
	go run $(PKG)

## test: run the test suite
test:
	go test ./...

## test-race: run tests with the race detector
test-race:
	go test -race ./...

## lint: run golangci-lint
lint:
	golangci-lint run ./...

## fmt: format the code
fmt:
	gofmt -s -w .

## vet: run go vet
vet:
	go vet ./...

## tidy: tidy go modules
tidy:
	go mod tidy

## clean: remove build artifacts
clean:
	rm -rf $(BIN_DIR)

## openapi: regenerate the OpenAPI 3.1 spec from handler annotations (swaggo/swag v2)
openapi:
	$(SWAG) init -g doc.go -d internal/api/gossignol -o internal/api/docs --ot json,yaml --parseInternal --v3.1

## openapi-check: fail if the committed spec is out of date
openapi-check: openapi
	git diff --exit-code internal/api/docs

## ci: the checks run in CI
ci: tidy vet lint test build openapi-check

## docker: build the docker image
docker:
	docker build -t gossignol/gossignol:$(VERSION) .

## docker-up: start the stack with docker compose
docker-up:
	docker compose up --build

## docker-down: stop the stack
docker-down:
	docker compose down
