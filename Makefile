VERSION ?= $(shell bun -p 'require("./package.json").version')
PREFIX ?= $(HOME)/.local/bin

.PHONY: build test check smoke edge-smoke live-smoke fuzz vuln install clean
build:
	go build -buildvcs=false -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o build/termbelt .
test:
	go test -race ./...
check: test
	go vet ./...
smoke: build
	python3 scripts/smoke.py --binary build/termbelt
edge-smoke: build
	python3 scripts/edge_smoke.py --binary build/termbelt
live-smoke: build
	python3 scripts/smoke.py --binary build/termbelt --live
fuzz:
	go test ./internal/core -run '^$$' -fuzz '^FuzzDomainNormalization$$' -fuzztime 10s -parallel 2
	go test ./internal/core -run '^$$' -fuzz '^FuzzTimestampParsing$$' -fuzztime 10s -parallel 2
vuln:
	go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
install: build
	mkdir -p "$(PREFIX)"
	@set -e; task_install_tmp="$$(mktemp "$(PREFIX)/.termbelt-install.XXXXXX")"; \
	trap 'rm -f "$$task_install_tmp"' EXIT; \
	install -m 755 build/termbelt "$$task_install_tmp"; \
	mv -f "$$task_install_tmp" "$(PREFIX)/termbelt"
clean:
	rm -rf build dist
