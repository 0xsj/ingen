package amberhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	amber "github.com/0xsj/ingen/amber"
)

func TestHeaderRoundTripAndContextPropagation(t *testing.T) {
	root, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}
	header, err := EncodeHeader(root)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(header, "{}\"") {
		t.Fatalf("header is not base64url encoded: %q", header)
	}

	decoded, present, err := DecodeHeader(header, amber.IncomingReject)
	if err != nil || !present || decoded.ExecutionID() != root.ExecutionID() {
		t.Fatalf("header round trip failed: present=%v err=%v", present, err)
	}

	headers := make(http.Header)
	headers.Set(HeaderName, header)
	ctx, present, err := WithIncomingHeaders(context.Background(), headers, amber.IncomingReject)
	if err != nil || !present {
		t.Fatalf("incoming header was not installed: present=%v err=%v", present, err)
	}
	current, ok := amber.ProvenanceFromContext(ctx)
	if !ok || current.ExecutionID() != root.ExecutionID() {
		t.Fatal("incoming context contains the wrong provenance")
	}
}

func TestMalformedAndAbsentHeadersFollowPolicy(t *testing.T) {
	if _, present, err := DecodeHeader("", amber.IncomingReject); err != nil || present {
		t.Fatalf("empty header should be absent: present=%v err=%v", present, err)
	}
	if _, present, err := DecodeHeader("not-base64", amber.IncomingIgnore); err != nil || present {
		t.Fatalf("invalid ignored header should be absent: present=%v err=%v", present, err)
	}
	if _, present, err := DecodeHeader("not-base64", amber.IncomingReject); err == nil || present {
		t.Fatalf("invalid rejected header should return an error: present=%v err=%v", present, err)
	}
	overlong := strings.Repeat("A", MaxEncodedHeaderBytes+1)
	if _, present, err := DecodeHeader(overlong, amber.IncomingIgnore); err != nil || present {
		t.Fatalf("oversized ignored header should be absent: present=%v err=%v", present, err)
	}
}

func TestOutgoingRequestDoesNotMutateSource(t *testing.T) {
	root, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}
	source, err := http.NewRequest(http.MethodGet, "https://example.test", nil)
	if err != nil {
		t.Fatal(err)
	}
	clone, err := WithOutgoingRequest(source, root)
	if err != nil {
		t.Fatal(err)
	}
	if source.Header.Get(HeaderName) != "" {
		t.Fatal("source request was mutated")
	}
	if clone.Header.Get(HeaderName) == "" {
		t.Fatal("cloned request has no provenance header")
	}

	missing, present, err := WithIncomingRequest(source, amber.IncomingReject)
	if err != nil || present || missing != source {
		t.Fatalf("missing request header should preserve request: present=%v err=%v", present, err)
	}
}

type httpConformanceCase struct {
	Name            string               `json:"name"`
	Value           string               `json:"value"`
	Policy          amber.IncomingPolicy `json:"policy"`
	ExpectedPresent bool                 `json:"expected_present"`
	ExpectError     bool                 `json:"expect_error"`
}

type httpConformanceFixture struct {
	Version    int                   `json:"version"`
	HeaderName string                `json:"header_name"`
	Valid      []httpConformanceCase `json:"valid"`
	Invalid    []httpConformanceCase `json:"invalid"`
}

func TestHTTPConformanceFixture(t *testing.T) {
	data, err := os.ReadFile("../../../conformance/http-v1.json")
	if err != nil {
		t.Fatalf("read HTTP fixture: %v", err)
	}
	var fixture httpConformanceFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode HTTP fixture: %v", err)
	}
	if fixture.Version != amber.Version || fixture.HeaderName != HeaderName {
		t.Fatalf("fixture header contract mismatch: version=%d header=%q", fixture.Version, fixture.HeaderName)
	}
	for _, testCase := range fixture.Valid {
		t.Run("valid/"+testCase.Name, func(t *testing.T) {
			provenance, present, err := DecodeHeader(testCase.Value, amber.IncomingReject)
			if err != nil || !present || provenance.WorkID() == "" {
				t.Fatalf("valid header rejected: present=%v err=%v", present, err)
			}
		})
	}
	for _, testCase := range fixture.Invalid {
		t.Run("invalid/"+testCase.Name, func(t *testing.T) {
			_, present, err := DecodeHeader(testCase.Value, testCase.Policy)
			if testCase.ExpectError {
				if err == nil || present {
					t.Fatalf("expected header error: present=%v err=%v", present, err)
				}
				return
			}
			if err != nil || present != testCase.ExpectedPresent {
				t.Fatalf("unexpected result: present=%v err=%v", present, err)
			}
		})
	}
}
