BINARY := dropwatch
GO ?= go
GOFMT ?= gofmt
VERSION ?= dev
REVISION ?= $(shell git rev-parse --short HEAD 2>/dev/null || printf none)
CREATED ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X github.com/Simplarix/dropwatch/internal/version.Version=$(VERSION) -X github.com/Simplarix/dropwatch/internal/version.Commit=$(REVISION) -X github.com/Simplarix/dropwatch/internal/version.BuildTime=$(CREATED)

.PHONY: help build build-check test test-race cover lint module-check verify smoke smoke-live run docker-build compose-check security security-source security-image clean

help:
	@printf '%s\n' \
		'make build        Build ./dropwatch with version metadata' \
		'make test         Run the deterministic test suite' \
		'make test-race    Run tests with the race detector' \
		'make cover        Write coverage.out' \
		'make lint         Check gofmt and run go vet' \
		'make verify       Verify modules, lint, race-test, and build' \
		'make smoke        Run the offline CLI/watcher/health smoke test' \
		'make smoke-live   Also exercise real IANA, RDAP, and DNS services' \
		'make run          Open the dashboard through go run' \
		'make docker-build Build the local production image' \
		'make compose-check Validate the hardened production Compose file' \
		'make security     Run source, dependency, config, and image scans' \
		'make clean        Remove local build artifacts'

build:
	$(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $(BINARY) ./cmd/dropwatch

test:
	$(GO) test ./...

test-race:
	CGO_ENABLED=1 $(GO) test -race ./...

cover:
	$(GO) test -coverprofile=coverage.out ./...

lint:
	test -z "$$($(GOFMT) -l .)"
	$(GO) vet ./...

build-check:
	CGO_ENABLED=0 $(GO) build ./...

module-check:
	$(GO) mod verify
	test -z "$$($(GO) mod tidy -diff)"

verify: module-check lint test-race build-check

smoke:
	GO="$(GO)" sh scripts/smoke-local.sh

smoke-live:
	GO="$(GO)" LIVE_RDAP=1 sh scripts/smoke-local.sh

run:
	$(GO) run ./cmd/dropwatch dashboard

docker-build:
	docker build --build-arg VERSION=$(VERSION) --build-arg REVISION=$(REVISION) --build-arg CREATED=$(CREATED) -t dropwatch:local .

compose-check:
	DROPWATCH_IMAGE=dropwatch:local docker compose -f compose.production.yml config --quiet

security: security-source security-image

security-source:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
	$(GO) run github.com/securego/gosec/v2/cmd/gosec@v2.28.0 -quiet ./...

security-image: docker-build
	GOEXPERIMENT=jsonv2 $(GO) run github.com/aquasecurity/trivy/cmd/trivy@v0.72.0 image --scanners vuln --severity UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL --exit-code 1 --no-progress dropwatch:local
	GOEXPERIMENT=jsonv2 $(GO) run github.com/aquasecurity/trivy/cmd/trivy@v0.72.0 config --severity HIGH,CRITICAL --exit-code 1 .

clean:
	$(GO) clean
	rm -f ./$(BINARY)
	rm -rf ./dist
