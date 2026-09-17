package governance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

const GitHubDefaultAPIBaseURL = "https://api.github.com"
const GitHubDefaultAPIVersion = "2026-03-10"

// GitHubTeamMembershipProvider reads one GitHub team and maps its direct
// members to one Hammond role. GitHub child-team members are ignored when
// inherited is true; role discovery and nested team expansion are deliberately
// outside this first adapter slice.
type GitHubTeamMembershipProvider struct {
	Organization string
	TeamSlug     string
	HammondRole  string
	APIBaseURL   string
	APIVersion   string

	Client            *http.Client
	Authenticate      MembershipRequestAuthenticator
	EndpointPolicy    MembershipEndpointPolicy
	AllowInsecureHTTP bool

	PerPage          int
	MaxPages         int
	MaxResponseBytes int64
}

// GitHubMembershipSnapshotRequest supplies the caller-owned identity and
// issuance metadata for one normalized snapshot. Version allocation belongs
// to the caller and may be coordinated by FileMembershipVersionStore.
type GitHubMembershipSnapshotRequest struct {
	ID          string
	Version     int
	ArtifactURI string
	IssuedAt    string
	ExpiresAt   string
}

type githubTeamMember struct {
	Login     string `json:"login"`
	ID        int64  `json:"id"`
	Role      string `json:"role"`
	Inherited bool   `json:"inherited"`
}

func (provider GitHubTeamMembershipProvider) Validate() error {
	if strings.TrimSpace(provider.Organization) == "" {
		return fmt.Errorf("GitHub organization is required")
	}
	if strings.ContainsAny(provider.Organization, "/?#@") {
		return fmt.Errorf("GitHub organization contains invalid URL characters")
	}
	if strings.TrimSpace(provider.TeamSlug) == "" {
		return fmt.Errorf("GitHub team slug is required")
	}
	if strings.ContainsAny(provider.TeamSlug, "/?#@") {
		return fmt.Errorf("GitHub team slug contains invalid URL characters")
	}
	if strings.TrimSpace(provider.HammondRole) == "" {
		return fmt.Errorf("Hammond role is required")
	}
	if provider.PerPage < 0 || provider.PerPage > 100 {
		return fmt.Errorf("GitHub members per_page must be between 1 and 100")
	}
	if provider.MaxPages < 0 {
		return fmt.Errorf("GitHub members max pages must not be negative")
	}
	if provider.MaxResponseBytes < 0 {
		return fmt.Errorf("GitHub members max response bytes must not be negative")
	}
	if _, err := provider.baseURL(); err != nil {
		return err
	}
	return nil
}

func (provider GitHubTeamMembershipProvider) baseURL() (*url.URL, error) {
	base := strings.TrimSpace(provider.APIBaseURL)
	if base == "" {
		base = GitHubDefaultAPIBaseURL
	}
	if err := validateMembershipEndpoint(base); err != nil {
		return nil, fmt.Errorf("GitHub API base URL is invalid: %w", err)
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("GitHub API base URL is invalid: %w", err)
	}
	if !provider.AllowInsecureHTTP && parsed.Scheme != "https" {
		return nil, fmt.Errorf("GitHub API base URL must use https")
	}
	return parsed, nil
}

func (request GitHubMembershipSnapshotRequest) validate() error {
	if strings.TrimSpace(request.ID) == "" {
		return fmt.Errorf("GitHub membership snapshot id is required")
	}
	if request.Version < 1 {
		return fmt.Errorf("GitHub membership snapshot version must be positive")
	}
	if strings.TrimSpace(request.ArtifactURI) == "" {
		return fmt.Errorf("GitHub membership snapshot artifact URI is required")
	}
	if _, err := parseUTC(request.IssuedAt); err != nil {
		return fmt.Errorf("GitHub membership snapshot issued_at must be RFC3339 UTC: %w", err)
	}
	if request.ExpiresAt != "" {
		issuedAt, _ := parseUTC(request.IssuedAt)
		expiresAt, err := parseUTC(request.ExpiresAt)
		if err != nil {
			return fmt.Errorf("GitHub membership snapshot expires_at must be RFC3339 UTC: %w", err)
		}
		if !expiresAt.After(issuedAt) {
			return fmt.Errorf("GitHub membership snapshot expires_at must be after issued_at")
		}
	}
	return nil
}

// FetchSnapshot retrieves all complete pages, signs the normalized Hammond
// envelope with the caller-owned issuer, and verifies the resulting bytes
// against the caller-owned trust set before returning them.
func (provider GitHubTeamMembershipProvider) FetchSnapshot(ctx context.Context, request GitHubMembershipSnapshotRequest, signer AuthoritySignatureSigner, verifier AuthoritySignatureVerifier) (MembershipSnapshot, error) {
	if err := provider.Validate(); err != nil {
		return MembershipSnapshot{}, err
	}
	if err := request.validate(); err != nil {
		return MembershipSnapshot{}, err
	}
	if signer == nil {
		return MembershipSnapshot{}, fmt.Errorf("GitHub membership signer is required")
	}
	if verifier == nil {
		return MembershipSnapshot{}, fmt.Errorf("GitHub membership signature verifier is required")
	}
	members, err := provider.fetchMembers(ctx)
	if err != nil {
		return MembershipSnapshot{}, err
	}
	grants := make([]AuthorityGrant, 0, len(members))
	for _, member := range members {
		if member.Inherited {
			continue
		}
		grants = append(grants, AuthorityGrant{
			Actor:     "github:user:" + strconv.FormatInt(member.ID, 10),
			Role:      provider.HammondRole,
			ValidFrom: request.IssuedAt,
		})
	}
	sort.Slice(grants, func(left, right int) bool {
		return grants[left].Actor < grants[right].Actor
	})
	document := membershipDocument{
		Schema:    MembershipSchema,
		ID:        request.ID,
		Version:   request.Version,
		IssuedAt:  request.IssuedAt,
		ExpiresAt: request.ExpiresAt,
		Grants:    grants,
	}
	payload, err := canonicalMembershipPayload(document)
	if err != nil {
		return MembershipSnapshot{}, fmt.Errorf("canonicalize GitHub membership: %w", err)
	}
	signature, err := signer.Sign(payload)
	if err != nil {
		return MembershipSnapshot{}, fmt.Errorf("sign GitHub membership: %w", err)
	}
	document.Signature = &signature
	data, err := json.Marshal(document)
	if err != nil {
		return MembershipSnapshot{}, fmt.Errorf("encode GitHub membership: %w", err)
	}
	digest := sha256.Sum256(data)
	reference := MembershipReference{
		ID:      request.ID,
		Version: request.Version,
		Schema:  MembershipSchema,
		Artifact: Artifact{
			URI:    request.ArtifactURI,
			SHA256: hex.EncodeToString(digest[:]),
		},
	}
	snapshot, err := DecodeMembershipSnapshotWithSignatureVerifier(data, reference, verifier)
	if err != nil {
		return MembershipSnapshot{}, fmt.Errorf("verify GitHub membership: %w", err)
	}
	return snapshot, nil
}

func (provider GitHubTeamMembershipProvider) fetchMembers(ctx context.Context) ([]githubTeamMember, error) {
	perPage := provider.PerPage
	if perPage == 0 {
		perPage = 100
	}
	maxPages := provider.MaxPages
	if maxPages == 0 {
		maxPages = 1000
	}
	seen := make(map[int64]struct{})
	members := make([]githubTeamMember, 0)
	for page := 1; page <= maxPages; page++ {
		pageMembers, err := provider.fetchPage(ctx, page, perPage)
		if err != nil {
			return nil, err
		}
		for _, member := range pageMembers {
			if member.ID <= 0 {
				return nil, fmt.Errorf("GitHub team member %q has invalid id", member.Login)
			}
			if strings.TrimSpace(member.Login) == "" {
				return nil, fmt.Errorf("GitHub team member %d has empty login", member.ID)
			}
			if member.Role != "member" && member.Role != "maintainer" {
				return nil, fmt.Errorf("GitHub team member %q has unsupported role %q", member.Login, member.Role)
			}
			if _, exists := seen[member.ID]; exists {
				return nil, fmt.Errorf("GitHub team member %d appeared more than once", member.ID)
			}
			seen[member.ID] = struct{}{}
			members = append(members, member)
		}
		if len(pageMembers) < perPage {
			return members, nil
		}
	}
	return nil, fmt.Errorf("GitHub team membership exceeded max pages %d; response may be incomplete", maxPages)
}

func (provider GitHubTeamMembershipProvider) fetchPage(ctx context.Context, page, perPage int) ([]githubTeamMember, error) {
	endpoint, err := provider.pageURL(page, perPage)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create GitHub membership request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	apiVersion := strings.TrimSpace(provider.APIVersion)
	if apiVersion == "" {
		apiVersion = GitHubDefaultAPIVersion
	}
	request.Header.Set("X-GitHub-Api-Version", apiVersion)
	request.Header.Set("User-Agent", "ingen-hammond")
	if provider.Authenticate != nil {
		if err := provider.Authenticate.Authenticate(request); err != nil {
			return nil, fmt.Errorf("authenticate GitHub membership request: %w", err)
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
			return fmt.Errorf("GitHub membership redirect rejected: %w", err)
		}
		if callerRedirectPolicy != nil {
			return callerRedirectPolicy(redirectRequest, via)
		}
		return nil
	}
	response, err := clientCopy.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch GitHub membership page %d: %w", page, err)
	}
	if response == nil {
		return nil, fmt.Errorf("fetch GitHub membership page %d: empty response", page)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub membership page %d returned HTTP status %d", page, response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, provider.maxResponseBytes()+1))
	if err != nil {
		return nil, fmt.Errorf("read GitHub membership page %d: %w", page, err)
	}
	if int64(len(data)) > provider.maxResponseBytes() {
		return nil, fmt.Errorf("GitHub membership page %d exceeds %d bytes", page, provider.maxResponseBytes())
	}
	var members []githubTeamMember
	if err := json.Unmarshal(data, &members); err != nil {
		return nil, fmt.Errorf("decode GitHub membership page %d: %w", page, err)
	}
	return members, nil
}

func (provider GitHubTeamMembershipProvider) pageURL(page, perPage int) (string, error) {
	base, err := provider.baseURL()
	if err != nil {
		return "", err
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/orgs/" + url.PathEscape(provider.Organization) + "/teams/" + url.PathEscape(provider.TeamSlug) + "/members"
	query := base.Query()
	query.Set("per_page", strconv.Itoa(perPage))
	query.Set("page", strconv.Itoa(page))
	query.Set("role", "all")
	base.RawQuery = query.Encode()
	endpoint := base.String()
	if err := provider.validateEndpoint(endpoint); err != nil {
		return "", err
	}
	return endpoint, nil
}

func (provider GitHubTeamMembershipProvider) validateEndpoint(endpoint string) error {
	if err := validateMembershipEndpoint(endpoint); err != nil {
		return err
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("GitHub membership endpoint is invalid: %w", err)
	}
	if !provider.AllowInsecureHTTP && parsed.Scheme != "https" {
		return fmt.Errorf("GitHub membership endpoint must use https")
	}
	if provider.EndpointPolicy != nil {
		if err := provider.EndpointPolicy(endpoint); err != nil {
			return fmt.Errorf("GitHub membership endpoint policy rejected endpoint: %w", err)
		}
	}
	return nil
}

func (provider GitHubTeamMembershipProvider) maxResponseBytes() int64 {
	if provider.MaxResponseBytes == 0 {
		return 1 << 20
	}
	return provider.MaxResponseBytes
}
