BIN     := enzo
PKG     := github.com/justin-efficient/enzo
DIST    := dist

# internal/version holds the released version and is the source of truth. A
# build from a tagged tree overrides it, so a binary cut from v0.1.0-3-gabc123
# says so instead of claiming to be the release. An untagged tree describes
# nothing, leaves VERSION empty, and keeps what is in source.
VERSION ?= $(shell git describe --tags --dirty 2>/dev/null | sed 's/^v//')
LDFLAGS := -s -w
ifneq ($(strip $(VERSION)),)
LDFLAGS += -X $(PKG)/internal/version.Version=$(VERSION)
endif

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
