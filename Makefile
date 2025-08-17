UBINDIR ?= /usr/bin
DESTDIR ?=
NAME = anise-repo-devkit
GOLANG_VERSION=$(shell go env GOVERSION)

override LDFLAGS += -X "github.com/macaroni-os/anise-repo-devkit/pkg/devkit.BuildTime=$(shell date -u '+%Y-%m-%d %I:%M:%S %Z')"
override LDFLAGS += -X "github.com/macaroni-os/anise-repo-devkit/pkg/devkit.BuildCommit=$(shell git rev-parse HEAD)"
override LDFLAGS += -X "github.com/macaroni-os/anise-repo-devkit/pkg/devkit.BuildGoVersion=$(GOLANG_VERSION)"

all: build install

build:
	CGO_ENABLED=0 go build -o $(NAME) -ldflags '$(LDFLAGS)' $(NAME).go

install: build
	install -d $(DESTDIR)/$(UBINDIR)
	install -m 0755 $(NAME) $(DESTDIR)/$(UBINDIR)/

.PHONY: build-small
build-small: build
	upx --brute -1 $(NAME)

.PHONY: vendor
vendor:
	go mod vendor

.PHONY: goreleaser-snapshot
goreleaser-snapshot:
	rm -rf dist/ || true
	goreleaser release --skip=validate,publish --snapshot --verbose

