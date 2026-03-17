.PHONY: all build test test-unit test-integration test-e2e test-coverage lint lint-fix arch-lint vet fmt clean deps check tools help

## all: Run full check (default)
all: check

## build: Build all packages
build:
	go build ./...

## test: Run all tests (excluding e2e)
test:
	go test ./... -v -count=1

## test-unit: Run unit tests only (internal/)
test-unit:
	go test ./internal/... -v -count=1

## test-integration: Run integration tests (handler)
test-integration:
	go test -v -count=1 -run TestHandler ./

## test-e2e: Run e2e tests (requires docker-compose stack)
test-e2e:
	go test -tags e2e ./examples/server/ -v -count=1

## test-coverage: Run tests with coverage report
test-coverage:
	go test ./... -coverprofile=coverage.out -count=1
	go tool cover -func=coverage.out
	@echo ""
	@echo "To view HTML report: go tool cover -html=coverage.out"

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## lint-fix: Run golangci-lint with auto-fix
lint-fix:
	golangci-lint run --fix ./...

## arch-lint: Run architecture dependency linter
arch-lint:
	go-arch-lint check

## vet: Run go vet
vet:
	go vet ./...

## fmt: Format code (goimports + golines)
fmt:
	goimports -w -local github.com/hasanMshawrab/bitslack .
	golines -w --max-len=120 .

## deps: Tidy go.mod
deps:
	go mod tidy

## check: Full check (build + vet + lint + arch-lint + test)
check: build vet lint arch-lint test

## clean: Clean build artifacts
clean:
	rm -f coverage.out
	go clean ./...

## tools: Install all required Go tools (golangci-lint, go-arch-lint, goimports, golines)
tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.8.0
	go install github.com/fe3dback/go-arch-lint@v1.14.0
	go install golang.org/x/tools/cmd/goimports@v0.41.0
	go install github.com/segmentio/golines@v0.13.0

## help: Display available commands
help:
	@echo 'bitslack commands:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'
