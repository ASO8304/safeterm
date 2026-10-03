.PHONY: build test e2e security vet fmt fmt-check verify offline-package clean

build:
	@echo "==> Building..."
	go build ./...

test:
	@echo "==> Running unit tests..."
	go test ./... -v -cover

e2e:
	@echo "==> Running deployment and public-boundary end-to-end tests..."
	go test ./internal/deployment ./tests/e2e -count=1 -v

security:
	@echo "==> Running adversarial security tests..."
	go test ./tests/security -count=1 -v
	@echo "==> Running race detector across all packages..."
	go test -race ./...

vet:
	@echo "==> Running go vet..."
	go vet ./...

fmt:
	gofmt -w .

fmt-check:
	@echo "==> Checking formatting..."
	@unformatted="$$(gofmt -l .)" || exit 1; \
	if [ -n "$$unformatted" ]; then \
		echo "Formatting issues found. Run 'make fmt'."; \
		printf '%s\n' "$$unformatted"; \
		exit 1; \
	fi

# The single command that must always stay green.
# Every new feature must keep 'make verify' passing before merge.
verify: fmt-check vet build test
	@echo ""
	@echo "SafeTerm Verification: PASS"

offline-package:
	@test -n "$(OUTPUT)" || { echo "usage: make offline-package OUTPUT=/path/to/package [ARCH=amd64|arm64]"; exit 2; }
	deploy/package/create-package.sh --output "$(OUTPUT)" $(if $(ARCH),--arch "$(ARCH)",)

clean:
	rm -rf bin/
