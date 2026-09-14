.PHONY: test check test-go test-typescript build-typescript

test: test-go test-typescript

check: test

test-go:
	cd go && go test ./...

test-typescript:
	cd typescript && npm test

build-typescript:
	cd typescript && npm run build
