.PHONY: generate check-generated check-auth-surface test vet vuln verify

generate:
	go generate ./internal/generate

check-generated:
	./scripts/check-generated.sh

check-auth-surface:
	@if grep -R -n -E 'sessionCookie|SessionCookie|CsrfHeader|X-ViaPost-Csrf' api; then \
		echo "generated client unexpectedly exposes session or CSRF authentication" >&2; \
		exit 1; \
	fi

test:
	go test ./... -race -count=1

vet:
	go vet ./...

vuln:
	go tool govulncheck ./...

verify: check-generated check-auth-surface test vet vuln
