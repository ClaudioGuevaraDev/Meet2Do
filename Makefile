BIN      := bin/meet2do
PKG      := ./cmd/meet2do
# Paquetes con cobertura exigida (ui y cmd requieren una terminal interactiva).
COVERPKG := ./internal/ai/... ./internal/output/... ./internal/recorder/... ./internal/transcriber/...
MINCOVER := 80

.PHONY: build run test test-race cover cover-check lint fmt tidy clean

build:
	go build -o $(BIN) $(PKG)

run:
	go run $(PKG)

test:
	go test ./...

test-race:
	go test -race ./...

cover:
	go test -coverprofile=coverage.out $(COVERPKG)
	go tool cover -func=coverage.out

cover-check: cover
	@go tool cover -func=coverage.out | awk '/^total:/ { sub("%", "", $$3); if ($$3 < $(MINCOVER)) { print "cobertura " $$3 "% < $(MINCOVER)%"; exit 1 } }'

lint:
	go vet ./...
	golangci-lint run

fmt:
	gofmt -w .
	go run golang.org/x/tools/cmd/goimports@latest -w .

tidy:
	go mod tidy
	go mod verify

clean:
	rm -rf bin coverage.out
