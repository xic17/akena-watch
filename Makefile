# Akena Watch — Makefile
# La versión se inyecta en el binario desde git (etiquetas v*).

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X main.version=$(VERSION)
GO       ?= go

.PHONY: build build-linux-amd64 build-linux-arm64 run test vet release docker clean

build:
	@mkdir -p bin
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/akena-watch .

# binarios portables para linux/amd64 (VPS, CloudPanel 2, Cloudflare)
build-linux-amd64:
	@mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/akena-watch-linux-amd64 .

# compila para ARM (Raspberry Pi, ARM VPS, etc.)
build-linux-arm64:
	@mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/akena-watch-linux-arm64 .

# construye todos los binarios de una release + checksums en dist/
release:
	@mkdir -p dist
	@for target in "linux amd64" "linux arm64" "darwin amd64" "darwin arm64"; do \
		set -- $$target; \
		echo "==> akena-watch-$$1-$$2 ($(VERSION))"; \
		CGO_ENABLED=0 GOOS=$$1 GOARCH=$$2 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/akena-watch-$$1-$$2 .; \
	done
	@cd dist && sha256sum akena-watch-* > SHA256SUMS
	@echo "Release $(VERSION) lista en dist/"

run: build
	./bin/akena-watch

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

docker:
	docker build -t akena-watch -f deploy/Dockerfile .

# Nota: no borra data/ (es la base de datos del usuario).
clean:
	rm -rf bin dist
