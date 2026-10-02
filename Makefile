.PHONY: build test security vet fmt fmt-check verify clean

build:
	@echo "==> Building..."
	go build ./...

test:
	@echo "==> Running unit tests..."
	go test ./... -v -cover

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

clean:
	rm -rf bin/
