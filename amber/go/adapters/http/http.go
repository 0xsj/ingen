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
	return ambertransport.DecodeValue(value, policy)
}

// WithIncomingHeaders inspects the Amber header and installs an accepted value
// in a derived context. Missing or ignored input returns the original context.
func WithIncomingHeaders(ctx context.Context, headers http.Header, policy amber.IncomingPolicy) (context.Context, bool, error) {
	if ctx == nil {
		return nil, false, fmt.Errorf("%w: context cannot be nil", amber.ErrInvalidTransition)
	}
	provenance, present, err := DecodeHeader(headers.Get(HeaderName), policy)
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
	if request == nil {
		return nil, false, fmt.Errorf("%w: request cannot be nil", amber.ErrInvalidTransition)
	}
	ctx, present, err := WithIncomingHeaders(request.Context(), request.Header, policy)
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
