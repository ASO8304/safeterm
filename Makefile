.PHONY: build test vet fmt fmt-check verify clean

build:
	@echo "==> Building..."
	go build ./...

test:
	@echo "==> Running unit tests..."
	go test ./... -v -cover

vet:
	@echo "==> Running go vet..."
	go vet ./...

fmt:
	gofmt -w .

fmt-check:
	@echo "==> Checking formatting..."
	@test -z "$$(gofmt -l .)" || (echo "Formatting issues found. Run 'make fmt'." && gofmt -l . && exit 1)

# The single command that must always stay green.
# Every new feature must keep 'make verify' passing before merge.
verify: fmt-check vet build test
	@echo ""
	@echo "SafeTerm Verification: PASS"

clean:
	rm -rf bin/
