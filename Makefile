GO ?= go

.PHONY: test test-unit test-race vet build deps check-deps

test: test-unit test-race vet build

test-unit:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

build:
	$(GO) build ./...

# The web UI's browser dependencies come from npm (web/ui/package.json). Install
# them before Go embeds the web UI. node_modules is not tracked; commit
# package.json and its lockfile.
deps:
	npm ci --prefix web/ui --no-audit --no-fund

check-deps: deps
	npm ls --prefix web/ui --depth=0
