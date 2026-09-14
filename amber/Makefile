.PHONY: test check lint package-check examples example-go example-typescript test-go test-typescript lint-go lint-typescript build-typescript

test: test-go test-typescript

check: lint test package-check

lint: lint-go lint-typescript

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

examples: example-go example-typescript

example-go:
	cd go && go run ./examples/compose

example-typescript:
	cd typescript && npm run example
