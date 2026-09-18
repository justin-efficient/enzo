BIN     := enzo
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
DIST    := dist

.PHONY: all build test race cover vet fmt tidy clean dist

all: vet test build

build:
	go build -ldflags '$(LDFLAGS)' -o $(BIN) .

test:
	go test ./...

race:
	go test -race -count=1 ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

vet:
	go vet ./...

fmt:
	gofmt -l -w .

tidy:
	go mod tidy

# Static linux binaries for both architectures.
dist: $(DIST)/$(BIN)-linux-amd64 $(DIST)/$(BIN)-linux-arm64
	cd $(DIST) && sha256sum $(BIN)-linux-* > checksums.txt

$(DIST)/$(BIN)-linux-%:
	@mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=$* go build -trimpath -ldflags '$(LDFLAGS)' -o $@ .

clean:
	rm -rf $(BIN) $(DIST) coverage.out
