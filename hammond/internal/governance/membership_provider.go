package governance

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// MembershipRequestAuthenticator lets the caller attach provider-specific
// credentials or proof to an outbound request. Hammond does not store or
// interpret those credentials.
type MembershipRequestAuthenticator interface {
	Authenticate(*http.Request) error
}

// MembershipRequestAuthenticatorFunc adapts a function to the authentication
// interface for simple per-request credentials.
type MembershipRequestAuthenticatorFunc func(*http.Request) error

func (authenticator MembershipRequestAuthenticatorFunc) Authenticate(request *http.Request) error {
	return authenticator(request)
}

// MembershipEndpointResolver lets the caller discover a provider endpoint at
// fetch time. Hammond validates only the returned HTTP(S) URL.
type MembershipEndpointResolver func(context.Context) (string, error)

// MembershipEndpointPolicy lets the caller enforce deployment-specific
// endpoint rules such as host allowlists or network tenancy.
type MembershipEndpointPolicy func(string) error

// MembershipSnapshotProvider fetches and verifies one normalized membership
// snapshot. Implementations own provider transport and authentication.
type MembershipSnapshotProvider interface {
	Fetch(context.Context, MembershipReference, AuthoritySignatureVerifier) (MembershipSnapshot, error)
}

// NormalizedMembershipResponse is the artifact produced by a caller-owned
// provider normalizer. Reference.Artifact.SHA256 must identify Data exactly.
type NormalizedMembershipResponse struct {
	Data      []byte
	Reference MembershipReference
}

// MembershipResponseNormalizer maps one provider-native response into a
// Hammond membership envelope. It owns provider-specific mapping and
// completeness decisions; Hammond verifies the returned envelope afterward.
type MembershipResponseNormalizer func(context.Context, string, []byte) (NormalizedMembershipResponse, error)

// HTTPMembershipProvider is a transport adapter for providers that return one
// complete Hammond membership snapshot over HTTP. The response remains
// untrusted until the existing digest and signature checks succeed.
type HTTPMembershipProvider struct {
	Endpoint         string
	ResolveEndpoint  MembershipEndpointResolver
	EndpointPolicy   MembershipEndpointPolicy
	RequireHTTPS     bool
	Client           *http.Client
	Authenticate     MembershipRequestAuthenticator
	MaxResponseBytes int64
}

func (provider HTTPMembershipProvider) Validate() error {
	if strings.TrimSpace(provider.Endpoint) == "" && provider.ResolveEndpoint == nil {
		return fmt.Errorf("membership provider endpoint or resolver is required")
	}
	if strings.TrimSpace(provider.Endpoint) != "" && provider.ResolveEndpoint != nil {
		return fmt.Errorf("membership provider endpoint and resolver are mutually exclusive")
	}
	if provider.MaxResponseBytes <= 0 {
		return fmt.Errorf("membership provider max response bytes must be positive")
	}
	if provider.ResolveEndpoint == nil {
		if err := validateMembershipEndpoint(provider.Endpoint); err != nil {
			return err
		}
	}
	return nil
}

func validateMembershipEndpoint(endpoint string) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return fmt.Errorf("membership provider resolved endpoint is required")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("membership provider endpoint is invalid: %w", err)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return fmt.Errorf("membership provider endpoint scheme must be http or https")
	}
	if parsed.Host == "" {
		return fmt.Errorf("membership provider endpoint host is required")
	}
	if parsed.User != nil {
		return fmt.Errorf("membership provider endpoint must not contain userinfo")
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("membership provider endpoint must not contain a fragment")
	}
	return nil
}

// Fetch gets exactly one provider response, bounds its size, and sends the
// bytes through Hammond's strict digest and issuer-signature loader.
func (provider HTTPMembershipProvider) Fetch(ctx context.Context, reference MembershipReference, verifier AuthoritySignatureVerifier) (MembershipSnapshot, error) {
	if err := provider.Validate(); err != nil {
		return MembershipSnapshot{}, err
	}
	if problems := validateMembershipReference(reference, "membership"); len(problems) > 0 {
		return MembershipSnapshot{}, fmt.Errorf("%s", joinProblems(problems))
	}
	if verifier == nil {
		return MembershipSnapshot{}, fmt.Errorf("membership provider signature verifier is required")
	}
	data, _, err := provider.fetchBytes(ctx)
	if err != nil {
		return MembershipSnapshot{}, err
	}
	snapshot, err := DecodeMembershipSnapshotWithSignatureVerifier(data, reference, verifier)
	if err != nil {
		return MembershipSnapshot{}, fmt.Errorf("verify membership provider response: %w", err)
	}
	return snapshot, nil
}

// FetchNormalized fetches one provider-native response, delegates mapping and
// completeness decisions to normalizer, and verifies the resulting Hammond
// membership envelope.
func (provider HTTPMembershipProvider) FetchNormalized(ctx context.Context, normalizer MembershipResponseNormalizer, verifier AuthoritySignatureVerifier) (MembershipSnapshot, error) {
	if err := provider.Validate(); err != nil {
		return MembershipSnapshot{}, err
	}
	if normalizer == nil {
		return MembershipSnapshot{}, fmt.Errorf("membership response normalizer is required")
	}
	if verifier == nil {
		return MembershipSnapshot{}, fmt.Errorf("membership provider signature verifier is required")
	}
	data, endpoint, err := provider.fetchBytes(ctx)
	if err != nil {
		return MembershipSnapshot{}, err
	}
	normalized, err := normalizer(ctx, endpoint, data)
	if err != nil {
		return MembershipSnapshot{}, fmt.Errorf("normalize membership provider response: %w", err)
	}
	if len(normalized.Data) == 0 {
		return MembershipSnapshot{}, fmt.Errorf("normalized membership response data is required")
	}
	snapshot, err := DecodeMembershipSnapshotWithSignatureVerifier(normalized.Data, normalized.Reference, verifier)
	if err != nil {
		return MembershipSnapshot{}, fmt.Errorf("verify normalized membership provider response: %w", err)
	}
	return snapshot, nil
}

func (provider HTTPMembershipProvider) fetchBytes(ctx context.Context) ([]byte, string, error) {
	if err := provider.Validate(); err != nil {
		return nil, "", err
	}
	endpoint, err := provider.resolveEndpoint(ctx)
	if err != nil {
		return nil, "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create membership provider request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if provider.Authenticate != nil {
		if err := provider.Authenticate.Authenticate(request); err != nil {
			return nil, "", fmt.Errorf("authenticate membership provider request: %w", err)
		}
	}
	client := provider.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, "", fmt.Errorf("fetch membership provider response: %w", err)
	}
	if response == nil {
		return nil, "", fmt.Errorf("fetch membership provider response: empty response")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("membership provider returned HTTP status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, provider.MaxResponseBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("read membership provider response: %w", err)
	}
	if int64(len(data)) > provider.MaxResponseBytes {
		return nil, "", fmt.Errorf("membership provider response exceeds %d bytes", provider.MaxResponseBytes)
	}
	return data, endpoint, nil
}

func (provider HTTPMembershipProvider) resolveEndpoint(ctx context.Context) (string, error) {
	endpoint := strings.TrimSpace(provider.Endpoint)
	if provider.ResolveEndpoint != nil {
		resolved, err := provider.ResolveEndpoint(ctx)
		if err != nil {
			return "", fmt.Errorf("resolve membership provider endpoint: %w", err)
		}
		endpoint = strings.TrimSpace(resolved)
	}
	if err := validateMembershipEndpoint(endpoint); err != nil {
		return "", err
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("membership provider endpoint is invalid: %w", err)
	}
	if provider.RequireHTTPS && parsed.Scheme != "https" {
		return "", fmt.Errorf("membership provider endpoint must use https")
	}
	if provider.EndpointPolicy != nil {
		if err := provider.EndpointPolicy(endpoint); err != nil {
			return "", fmt.Errorf("membership provider endpoint policy rejected endpoint: %w", err)
		}
	}
	return endpoint, nil
}

// FetchVerifierAt fetches a snapshot and applies the caller's freshness policy
// before returning a verifier suitable for policy evaluation.
func (provider HTTPMembershipProvider) FetchVerifierAt(ctx context.Context, reference MembershipReference, verifier AuthoritySignatureVerifier, now string, maxAge, maxFutureSkew time.Duration) (TimeScopedAuthority, error) {
	snapshot, err := provider.Fetch(ctx, reference, verifier)
	if err != nil {
		return TimeScopedAuthority{}, err
	}
	return snapshot.VerifierAt(now, maxAge, maxFutureSkew)
}
