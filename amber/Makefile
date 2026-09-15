.PHONY: test check release-check lint fuzz race package-check module-check postgres-integration postgres-schema-check examples example-go example-go-service example-typescript example-otel-go example-otel-typescript test-go test-typescript lint-go lint-typescript build-typescript

test: test-go test-typescript

check: lint test package-check module-check

release-check: check race fuzz examples

lint: lint-go lint-typescript

fuzz:
	cd go && go test . -run '^$$' -fuzz FuzzFromJSON -fuzztime 5s
	cd go && go test ./adapters/transport -run '^$$' -fuzz FuzzDecodeValue -fuzztime 5s

race:
	cd go && go test -race ./...

test-go:
	cd go && go test ./...

test-typescript:
	cd typescript && npm test

lint-go:
	cd go && go vet ./...

lint-typescript:
	cd typescript && npm run typecheck

build-typescript:
	cd typescript && npm run build

package-check:
	cd typescript && npm run build && npm pack --dry-run
	cd typescript && temp_dir=$$(mktemp -d) && \
		trap 'rm -rf "$$temp_dir"' EXIT && \
		package_name=$$(npm pack --silent --pack-destination "$$temp_dir") && \
		node test/package-smoke.mjs "$$temp_dir/$$package_name"

module-check:
	amber_dir=$$(pwd)/go && temp_dir=$$(mktemp -d) && \
		trap 'rm -rf "$$temp_dir"' EXIT && \
		cp go/examples/consumer/main.go "$$temp_dir/main.go" && \
		cd "$$temp_dir" && \
		go mod init example.com/amber-consumer && \
		go mod edit -replace github.com/0xsj/ingen/amber="$$amber_dir" && \
		go mod tidy && \
		go run .

postgres-integration:
	@test -n "$$AMBER_POSTGRES_DSN" || (echo "AMBER_POSTGRES_DSN must be set" && exit 1)
	cd go && go test ./adapters/storage -run '^TestPostgresStoreLive$$' -count=1

postgres-schema-check:
	@test -n "$$AMBER_POSTGRES_DSN" || (echo "AMBER_POSTGRES_DSN must be set" && exit 1)
	cd go && go run ./examples/postgres

examples: example-go example-go-service example-typescript example-otel-go example-otel-typescript

example-go:
	cd go && go run ./examples/compose

example-go-service:
	cd go && go run ./examples/service

example-typescript:
	cd typescript && npm run example

example-otel-go:
	cd go && go run ./examples/otel

example-otel-typescript:
	cd typescript && npm run example-otel
