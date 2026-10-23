BIN      := bin/meet2do
PKG      := ./cmd/meet2do
# Paquetes con cobertura exigida (ui y cmd requieren una terminal interactiva).
COVERPKG := ./internal/ai/... ./internal/output/... ./internal/recorder/... ./internal/transcriber/... ./internal/uninstall/...
# Código que solo se puede probar con un servidor de audio real (ver test-integration).
COVEREXCLUDE := /internal/recorder/pulse.go:
MINCOVER := 80

.PHONY: build dist run setup test test-race test-integration cover cover-check lint fmt tidy clean

# Compila meet2do y su atajo m2do (el mismo programa).
build:
	go build -o $(BIN) $(PKG)
	go build -o bin/m2do ./cmd/m2do

# Arquitecturas que se publican en GitHub Releases (las que descarga install.sh).
DIST_ARCHS := amd64 arm64

# Binarios estáticos (sin cgo) para el release: dist/meet2do-linux-<arch>.tar.gz y checksums.txt.
# Los nombres no llevan la versión para que releases/latest/download/<archivo> siempre sirva.
# m2do no se empaqueta: install.sh lo crea como symlink a meet2do.
dist:
	rm -rf dist && mkdir -p dist
	for arch in $(DIST_ARCHS); do \
		CGO_ENABLED=0 GOOS=linux GOARCH=$$arch go build -trimpath -ldflags "-s -w" -o dist/$$arch/meet2do $(PKG) && \
		tar -C dist/$$arch -czf dist/meet2do-linux-$$arch.tar.gz meet2do && \
		rm -r dist/$$arch || exit 1; \
	done
	cd dist && sha256sum meet2do-linux-*.tar.gz > checksums.txt

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
	rm -rf bin dist coverage.raw coverage.out
