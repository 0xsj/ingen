package governance

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestGitHubTeamMembershipProviderFetchesCompleteDirectMembership(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	var requestedPages []int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		page, err := strconv.Atoi(request.URL.Query().Get("page"))
		if err != nil {
			t.Fatalf("page query = %q: %v", request.URL.Query().Get("page"), err)
		}
		requestedPages = append(requestedPages, page)
		if request.URL.Path != "/orgs/ingen/teams/reviewers/members" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		if request.URL.Query().Get("per_page") != "2" || request.URL.Query().Get("role") != "all" {
			t.Fatalf("query = %v", request.URL.Query())
		}
		if request.Header.Get("Accept") != "application/vnd.github+json" || request.Header.Get("X-GitHub-Api-Version") == "" {
			t.Fatalf("GitHub headers = %#v", request.Header)
		}
		if request.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		pages := map[int][]githubTeamMember{
			1: {{Login: "direct-one", ID: 101, Role: "member"}, {Login: "child-one", ID: 202, Role: "member", Inherited: true}},
			2: {{Login: "direct-two", ID: 303, Role: "maintainer"}, {Login: "child-two", ID: 404, Role: "member", Inherited: true}},
			3: {},
		}
		if err := json.NewEncoder(writer).Encode(pages[page]); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	provider := GitHubTeamMembershipProvider{
		Organization:      "ingen",
		TeamSlug:          "reviewers",
		HammondRole:       "product-reviewer",
		APIBaseURL:        server.URL,
		AllowInsecureHTTP: true,
		PerPage:           2,
		Authenticate: MembershipRequestAuthenticatorFunc(func(request *http.Request) error {
			request.Header.Set("Authorization", "Bearer test-token")
			return nil
		}),
	}
	snapshot, err := provider.FetchSnapshot(context.Background(), GitHubMembershipSnapshotRequest{
		ID:          "github-ingen-reviewers",
		Version:     4,
		ArtifactURI: "github://ingen/team/reviewers",
		IssuedAt:    "2026-09-15T00:00:00Z",
	}, Ed25519AuthoritySignatureSigner{KeyID: "github-issuer-2026", PrivateKey: privateKey}, Ed25519AuthoritySignatureVerifier{
		Keys: map[string]ed25519.PublicKey{"github-issuer-2026": publicKey},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join([]string{strconv.Itoa(requestedPages[0]), strconv.Itoa(requestedPages[1]), strconv.Itoa(requestedPages[2])}, ",") != "1,2,3" {
		t.Fatalf("requested pages = %v, want [1 2 3]", requestedPages)
	}
	if len(snapshot.Grants) != 2 {
		t.Fatalf("grants = %#v, want two direct members", snapshot.Grants)
	}
	if snapshot.Grants[0].Actor != "github:user:101" || snapshot.Grants[1].Actor != "github:user:303" {
		t.Fatalf("grants = %#v, want stable GitHub user actors", snapshot.Grants)
	}
	for _, grant := range snapshot.Grants {
		if grant.Role != "product-reviewer" || grant.ValidFrom != "2026-09-15T00:00:00Z" {
			t.Fatalf("grant = %#v", grant)
		}
	}

	membershipVerifier, err := snapshot.VerifierAtWithProvenance("2026-09-15T00:30:00Z", 2*time.Hour, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if authorized, err := membershipVerifier.Verify("github:user:101", "product-reviewer", "2026-09-15T00:30:00Z"); err != nil || !authorized {
		t.Fatalf("direct member authorization = %v, %v", authorized, err)
	}
	if authorized, err := membershipVerifier.Verify("github:user:202", "product-reviewer", "2026-09-15T00:30:00Z"); err != nil || authorized {
		t.Fatalf("inherited member authorization = %v, %v; want denied", authorized, err)
	}
	if !membershipVerifier.MembershipReference().Equal(snapshot.Reference) {
		t.Fatalf("provenance = %#v, want snapshot reference %#v", membershipVerifier.MembershipReference(), snapshot.Reference)
	}

	policy := DefaultReviewPolicy()
	policy.AuthorityVerifier = membershipVerifier
	record := approvedRecord(strings.Repeat("a", 64))
	record.Events[2].Actor = "github:user:101"
	record.Events[2].Role = "product-reviewer"
	record.Events[2].Membership = &snapshot.Reference
	if err := record.ValidateWithPolicy(policy); err != nil {
		t.Fatalf("GitHub membership policy authorization = %v", err)
	}
}

func TestGitHubTeamMembershipProviderRejectsIncompletePagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		page := request.URL.Query().Get("page")
		_, _ = writer.Write([]byte(`[{"login":"member-` + page + `","id":` + page + `,"role":"member"}]`))
	}))
	defer server.Close()
	provider := GitHubTeamMembershipProvider{
		Organization:      "ingen",
		TeamSlug:          "reviewers",
		HammondRole:       "product-reviewer",
		APIBaseURL:        server.URL,
		AllowInsecureHTTP: true,
		PerPage:           1,
		MaxPages:          2,
	}
	_, err := provider.fetchMembers(context.Background())
	if err == nil || !strings.Contains(err.Error(), "response may be incomplete") {
		t.Fatalf("incomplete pagination error = %v", err)
	}
}

func TestGitHubTeamMembershipProviderRequiresHTTPSByDefault(t *testing.T) {
	provider := GitHubTeamMembershipProvider{
		Organization: "ingen",
		TeamSlug:     "reviewers",
		HammondRole:  "product-reviewer",
		APIBaseURL:   "http://github.example.test",
	}
	if err := provider.Validate(); err == nil || !strings.Contains(err.Error(), "must use https") {
		t.Fatalf("HTTPS validation error = %v", err)
	}
}
