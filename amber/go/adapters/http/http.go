// Package amberhttp provides HTTP header propagation for Amber provenance.
package amberhttp

import (
	"context"
	"fmt"
	"net/http"

	amber "github.com/0xsj/ingen/amber"
	ambertransport "github.com/0xsj/ingen/amber/adapters/transport"
)

const (
	HeaderName            = ambertransport.ProvenanceField
	MaxEncodedHeaderBytes = ambertransport.MaxEncodedValueBytes
)

// EncodeHeader serializes provenance as canonical UTF-8 JSON and encodes it
// as unpadded base64url for use as an HTTP header value.
func EncodeHeader(provenance amber.Provenance) (string, error) {
	return ambertransport.EncodeValue(provenance)
}

// DecodeHeader inspects an HTTP header value without installing it into a
// context. Empty input is absent; malformed input follows policy.
func DecodeHeader(value string, policy amber.IncomingPolicy) (amber.Provenance, bool, error) {
	return DecodeHeaderWithValidator(value, policy, nil)
}

// DecodeHeaderWithValidator inspects an HTTP header and applies an optional
// trust validator before the value can be installed into context.
func DecodeHeaderWithValidator(value string, policy amber.IncomingPolicy, validator amber.IncomingValidator) (amber.Provenance, bool, error) {
	provenance, present, err := ambertransport.DecodeValue(value, policy)
	if err != nil || !present || validator == nil {
		return provenance, present, err
	}
	if err := validator(provenance); err != nil {
		if policy == amber.IncomingIgnore {
			return amber.Provenance{}, false, nil
		}
		return amber.Provenance{}, false, fmt.Errorf("%w: incoming validation failed: %v", amber.ErrInvalidProvenance, err)
	}
	return provenance, true, nil
}

// WithIncomingHeaders inspects the Amber header and installs an accepted value
// in a derived context. Missing or ignored input returns the original context.
func WithIncomingHeaders(ctx context.Context, headers http.Header, policy amber.IncomingPolicy) (context.Context, bool, error) {
	return WithIncomingHeadersWithValidator(ctx, headers, policy, nil)
}

// WithIncomingHeadersWithValidator installs a header only after the optional
// trust validator accepts the decoded provenance.
func WithIncomingHeadersWithValidator(ctx context.Context, headers http.Header, policy amber.IncomingPolicy, validator amber.IncomingValidator) (context.Context, bool, error) {
	if ctx == nil {
		return nil, false, fmt.Errorf("%w: context cannot be nil", amber.ErrInvalidTransition)
	}
	provenance, present, err := DecodeHeaderWithValidator(headers.Get(HeaderName), policy, validator)
	if err != nil || !present {
		return ctx, present, err
	}
	installed, err := amber.WithProvenance(ctx, provenance)
	if err != nil {
		return ctx, false, err
	}
	return installed, true, nil
}

// WithIncomingRequest derives a request context from the request's Amber
// header. The request itself is left unchanged when the header is absent.
func WithIncomingRequest(request *http.Request, policy amber.IncomingPolicy) (*http.Request, bool, error) {
	return WithIncomingRequestWithValidator(request, policy, nil)
}

// WithIncomingRequestWithValidator derives a request context after the
// optional trust validator accepts the incoming header.
func WithIncomingRequestWithValidator(request *http.Request, policy amber.IncomingPolicy, validator amber.IncomingValidator) (*http.Request, bool, error) {
	if request == nil {
		return nil, false, fmt.Errorf("%w: request cannot be nil", amber.ErrInvalidTransition)
	}
	ctx, present, err := WithIncomingHeadersWithValidator(request.Context(), request.Header, policy, validator)
	if err != nil || !present {
		return request, present, err
	}
	return request.WithContext(ctx), true, nil
}

// SetOutgoingHeader adds provenance to an existing mutable header map.
func SetOutgoingHeader(headers http.Header, provenance amber.Provenance) error {
	if headers == nil {
		return fmt.Errorf("%w: headers cannot be nil", amber.ErrInvalidTransition)
	}
	value, err := EncodeHeader(provenance)
	if err != nil {
		return err
	}
	headers.Set(HeaderName, value)
	return nil
}

// WithOutgoingRequest returns a cloned request with provenance set on the
// clone. The source request and its headers are not mutated.
func WithOutgoingRequest(request *http.Request, provenance amber.Provenance) (*http.Request, error) {
	if request == nil {
		return nil, fmt.Errorf("%w: request cannot be nil", amber.ErrInvalidTransition)
	}
	clone := request.Clone(request.Context())
	if clone.Header == nil {
		clone.Header = make(http.Header)
	}
	if err := SetOutgoingHeader(clone.Header, provenance); err != nil {
		return nil, err
	}
	return clone, nil
}

// ErrorHandler handles malformed incoming provenance rejected by Middleware.
// It receives the original request so applications can add their own error
// response details without changing the propagation behavior.
type ErrorHandler func(http.ResponseWriter, *http.Request, error)

// DefaultErrorHandler returns a generic bad-request response for malformed
// incoming provenance. It intentionally does not expose parser details.
func DefaultErrorHandler(writer http.ResponseWriter, _ *http.Request, _ error) {
	http.Error(writer, "invalid Amber provenance", http.StatusBadRequest)
}

// Middleware installs accepted incoming provenance on the request context and
// copies the resulting context value to the response header. Missing input is
// left absent; rejected input receives a 400 response and does not reach next.
func Middleware(next http.Handler, policy amber.IncomingPolicy) http.Handler {
	return MiddlewareWithValidatorAndErrorHandler(next, policy, nil, nil)
}

// MiddlewareWithErrorHandler is Middleware with an application-defined error
// response for rejected incoming provenance.
func MiddlewareWithErrorHandler(next http.Handler, policy amber.IncomingPolicy, onError ErrorHandler) http.Handler {
	return MiddlewareWithValidatorAndErrorHandler(next, policy, nil, onError)
}

// MiddlewareWithValidator installs a caller-provided trust validator at the
// HTTP boundary while retaining the default 400 error response.
func MiddlewareWithValidator(next http.Handler, policy amber.IncomingPolicy, validator amber.IncomingValidator) http.Handler {
	return MiddlewareWithValidatorAndErrorHandler(next, policy, validator, nil)
}

// MiddlewareWithValidatorAndErrorHandler combines trust validation with a
// caller-defined error response for rejected input.
func MiddlewareWithValidatorAndErrorHandler(next http.Handler, policy amber.IncomingPolicy, validator amber.IncomingValidator, onError ErrorHandler) http.Handler {
	if next == nil {
		panic("amberhttp: next handler cannot be nil")
	}
	if onError == nil {
		onError = DefaultErrorHandler
	}

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		incoming, _, err := WithIncomingRequestWithValidator(request, policy, validator)
		if err != nil {
			onError(writer, request, err)
			return
		}

		if provenance, ok := amber.ProvenanceFromContext(incoming.Context()); ok {
			if err := SetOutgoingHeader(writer.Header(), provenance); err != nil {
				onError(writer, incoming, err)
				return
			}
		}
		next.ServeHTTP(writer, incoming)
	})
}
