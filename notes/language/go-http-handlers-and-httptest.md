# Go's HTTP handler boundary supports both serving and isolated testing

Go's `http.Handler` interface lets one boundary implementation run under a real server and under in-memory `httptest` requests.

## Origin

The first Go document-pipeline subject, using Go 1.27.1 on macOS. The same
subject must be runnable by a person and testable without opening a network
socket.

## What

An HTTP handler is a value with a `ServeHTTP(http.ResponseWriter, *http.Request)`
method. `net/http` can pass that value to `ListenAndServe`, while
`httptest.NewRequest` and `httptest.NewRecorder` can call it directly. This
keeps the transport boundary identical in local tests and in the running
subject.

## Why

Testing only through a live server introduces port allocation and process
lifecycle into every unit of feedback. Testing only helper functions skips the
actual method, path, status, header, and JSON boundary that Sorna will observe.

## Example

```go
handler := NewHandler(NewStore())
request := httptest.NewRequest(http.MethodGet, "/documents/doc-1", nil)
response := httptest.NewRecorder()
handler.ServeHTTP(response, request)
```

## Gotchas

- Set headers before writing the status or body; the response is committed on
  the first write.
- `httptest` proves handler behavior, not that a separately compiled binary
  starts correctly.
- A handler should not rely on test-only state if it is intended to be the
  subject of black-box verification.

## Used in

- [`document-pipeline subject`](../../examples/document-pipeline-lab/subject/)
- the future Sorna HTTP/JSON adapter

## Related

- [`The document subject keeps workflow transitions visible at the boundary`](../modules/document-pipeline-subject.md)
- [Go `net/http` package documentation](https://pkg.go.dev/net/http)
