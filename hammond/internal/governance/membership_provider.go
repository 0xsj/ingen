package governance

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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

// MembershipBearerTokenSource supplies a short-lived bearer token for one
// request. Token acquisition and storage remain caller-owned.
type MembershipBearerTokenSource func(context.Context) (string, error)

// BearerTokenAuthenticator is a generic request authenticator. It retains
// only the caller's token source, not a bearer token.
type BearerTokenAuthenticator struct {
	Source MembershipBearerTokenSource
}

func (authenticator BearerTokenAuthenticator) Authenticate(request *http.Request) error {
	if authenticator.Source == nil {
		return fmt.Errorf("membership bearer token source is required")
	}
	token, err := authenticator.Source(request.Context())
	if err != nil {
		return fmt.Errorf("load membership bearer token: %w", err)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("membership bearer token is required")
	}
	if strings.ContainsAny(token, "\r\n") {
		return fmt.Errorf("membership bearer token must not contain newlines")
	}
	request.Header.Set("Authorization", "Bearer "+token)
	return nil
}

// MembershipEndpointResolver lets the caller discover a provider endpoint at
// fetch time. Hammond validates only the returned HTTP(S) URL.
type MembershipEndpointResolver func(context.Context) (string, error)

// MembershipEndpointPolicy lets the caller enforce deployment-specific
// endpoint rules such as host allowlists or network tenancy.
type MembershipEndpointPolicy func(string) error

// MembershipEndpointAllowlist is a provider-neutral exact host and port
// policy. Hosts are compared case-insensitively after trimming one trailing
// dot; wildcard hosts are not supported. Ports are the effective URL ports,
// so an https URL without an explicit port is checked as 443 and an http URL
// without one as 80.
type MembershipEndpointAllowlist struct {
	Hosts []string
	Ports []int
}

// Validate checks an endpoint against the explicit host and port allowlist.
// The method is suitable for use as HTTPMembershipProvider.EndpointPolicy.
func (allowlist MembershipEndpointAllowlist) Validate(endpoint string) error {
	if len(allowlist.Hosts) == 0 {
		return fmt.Errorf("membership endpoint allowlist hosts are required")
	}
	if len(allowlist.Ports) == 0 {
		return fmt.Errorf("membership endpoint allowlist ports are required")
	}

	hosts := make(map[string]struct{}, len(allowlist.Hosts))
	for _, host := range allowlist.Hosts {
		host = normalizeMembershipEndpointHost(host)
		if host == "" || strings.ContainsAny(host, " /?#@*") {
			return fmt.Errorf("membership endpoint allowlist host %q is invalid", host)
		}
		if _, exists := hosts[host]; exists {
			return fmt.Errorf("membership endpoint allowlist host %q is duplicated", host)
		}
		hosts[host] = struct{}{}
	}

	ports := make(map[int]struct{}, len(allowlist.Ports))
	for _, port := range allowlist.Ports {
		if port < 1 || port > 65535 {
			return fmt.Errorf("membership endpoint allowlist port %d is invalid", port)
		}
		if _, exists := ports[port]; exists {
			return fmt.Errorf("membership endpoint allowlist port %d is duplicated", port)
		}
		ports[port] = struct{}{}
	}

	if err := validateMembershipEndpoint(endpoint); err != nil {
		return err
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("membership provider endpoint is invalid: %w", err)
	}
	host := normalizeMembershipEndpointHost(parsed.Hostname())
	if _, allowed := hosts[host]; !allowed {
		return fmt.Errorf("membership provider endpoint host %q is not allowlisted", parsed.Hostname())
	}
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	effectivePort, err := strconv.Atoi(port)
	if err != nil || effectivePort < 1 || effectivePort > 65535 {
		return fmt.Errorf("membership provider endpoint port %q is invalid", port)
	}
	if _, allowed := ports[effectivePort]; !allowed {
		return fmt.Errorf("membership provider endpoint port %d is not allowlisted", effectivePort)
	}
	return nil
}

func normalizeMembershipEndpointHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

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
	clientCopy := *client
	callerRedirectPolicy := clientCopy.CheckRedirect
	clientCopy.CheckRedirect = func(redirectRequest *http.Request, via []*http.Request) error {
		if err := provider.validateEndpoint(redirectRequest.URL.String()); err != nil {
			return fmt.Errorf("membership redirect endpoint rejected: %w", err)
		}
		if callerRedirectPolicy != nil {
			return callerRedirectPolicy(redirectRequest, via)
		}
		return nil
	}
	response, err := clientCopy.Do(request)
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
	sourceEndpoint := endpoint
	if response.Request != nil && response.Request.URL != nil {
		sourceEndpoint = response.Request.URL.String()
	}
	return data, sourceEndpoint, nil
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
	if err := provider.validateEndpoint(endpoint); err != nil {
		return "", err
	}
	return endpoint, nil
}

func (provider HTTPMembershipProvider) validateEndpoint(endpoint string) error {
	endpoint = strings.TrimSpace(endpoint)
	if err := validateMembershipEndpoint(endpoint); err != nil {
		return err
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("membership provider endpoint is invalid: %w", err)
	}
	if provider.RequireHTTPS && parsed.Scheme != "https" {
		return fmt.Errorf("membership provider endpoint must use https")
	}
	if provider.EndpointPolicy != nil {
		if err := provider.EndpointPolicy(endpoint); err != nil {
			return fmt.Errorf("membership provider endpoint policy rejected endpoint: %w", err)
		}
	}
	return nil
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

// FetchVerifierAtWithProvenance fetches a snapshot, applies freshness policy,
// and returns a verifier that can be matched to decision-event provenance.
func (provider HTTPMembershipProvider) FetchVerifierAtWithProvenance(ctx context.Context, reference MembershipReference, verifier AuthoritySignatureVerifier, now string, maxAge, maxFutureSkew time.Duration) (MembershipVerifier, error) {
	snapshot, err := provider.Fetch(ctx, reference, verifier)
	if err != nil {
		return MembershipVerifier{}, err
	}
	return snapshot.VerifierAtWithProvenance(now, maxAge, maxFutureSkew)
}
