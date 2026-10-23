BIN      := bin/meet2do
PKG      := ./cmd/meet2do
# Paquetes con cobertura exigida (ui y cmd requieren una terminal interactiva).
COVERPKG := ./internal/ai/... ./internal/output/... ./internal/recorder/... ./internal/transcriber/...
# Código que solo se puede probar con un servidor de audio real (ver test-integration).
COVEREXCLUDE := /internal/recorder/pulse.go:
MINCOVER := 80

.PHONY: build run setup test test-race test-integration cover cover-check lint fmt tidy clean

build:
	go build -o $(BIN) $(PKG)

run:
	go run $(PKG)

setup:
	go run $(PKG) -setup

test:
	go test ./...

test-race:
	go test -race ./...

# Graba unos segundos con PulseAudio/PipeWire real (no corre en CI).
test-integration:
	go test -tags integration -count=1 -v ./internal/recorder/

cover:
	go test -coverprofile=coverage.raw $(COVERPKG)
	grep -v '$(COVEREXCLUDE)' coverage.raw > coverage.out
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
	rm -rf bin coverage.raw coverage.out
