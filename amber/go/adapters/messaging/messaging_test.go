package ambermessaging

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	amber "github.com/0xsj/ingen/amber"
	ambertransport "github.com/0xsj/ingen/amber/adapters/transport"
)

func TestMetadataPropagationAndClone(t *testing.T) {
	root, err := amber.Start()
	if err != nil {
		t.Fatal(err)
	}
	source := Metadata{"trace-id": "trace-1"}
	metadata, err := WithOutgoingMetadata(source, root)
	if err != nil {
		t.Fatal(err)
	}
	if source[ProvenanceKey] != "" || metadata[ProvenanceKey] == "" {
		t.Fatal("outgoing metadata must be cloned")
	}
	decoded, present, err := DecodeMetadata(metadata, amber.IncomingReject)
	if err != nil || !present || decoded.ExecutionID() != root.ExecutionID() {
		t.Fatalf("metadata round trip failed: present=%v err=%v", present, err)
	}

	ctx, present, err := WithIncomingMetadata(context.Background(), metadata, amber.IncomingReject)
	if err != nil || !present {
		t.Fatalf("incoming metadata was not installed: present=%v err=%v", present, err)
	}
	current, ok := amber.ProvenanceFromContext(ctx)
	if !ok || current.ExecutionID() != root.ExecutionID() {
		t.Fatal("incoming metadata contains the wrong provenance")
	}
}

func TestMetadataPoliciesAndAbsentValues(t *testing.T) {
	if _, present, err := DecodeMetadata(nil, amber.IncomingReject); err != nil || present {
		t.Fatalf("missing metadata should be absent: present=%v err=%v", present, err)
	}
	if _, present, err := DecodeMetadata(Metadata{ProvenanceKey: "bad!"}, amber.IncomingIgnore); err != nil || present {
		t.Fatalf("invalid ignored metadata should be absent: present=%v err=%v", present, err)
	}
	if _, present, err := DecodeMetadata(Metadata{ProvenanceKey: "bad!"}, amber.IncomingReject); err == nil || present {
		t.Fatalf("invalid rejected metadata should return an error: present=%v err=%v", present, err)
	}
	if _, err := WithOutgoingMetadata(Metadata(nil), amber.Provenance{}); err == nil {
		t.Fatal("invalid provenance must not be encoded")
	}
	if _, present, err := DecodeMetadata(Metadata{ProvenanceKey: strings.Repeat("A", ambertransport.MaxEncodedValueBytes+1)}, amber.IncomingIgnore); err != nil || present {
		t.Fatalf("oversized ignored metadata should be absent: present=%v err=%v", present, err)
	}
}

type messagingConformanceCase struct {
	Name            string               `json:"name"`
	Metadata        map[string]string    `json:"metadata"`
	Policy          amber.IncomingPolicy `json:"policy"`
	ExpectedPresent bool                 `json:"expected_present"`
	ExpectError     bool                 `json:"expect_error"`
}

type messagingConformanceFixture struct {
	Version     int                        `json:"version"`
	MetadataKey string                     `json:"metadata_key"`
	Valid       []messagingConformanceCase `json:"valid"`
	Invalid     []messagingConformanceCase `json:"invalid"`
}

func TestMessagingConformanceFixture(t *testing.T) {
	data, err := os.ReadFile("../../../conformance/messaging-v1.json")
	if err != nil {
		t.Fatalf("read messaging fixture: %v", err)
	}
	var fixture messagingConformanceFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode messaging fixture: %v", err)
	}
	if fixture.Version != amber.Version || fixture.MetadataKey != ProvenanceKey {
		t.Fatalf("fixture metadata contract mismatch: version=%d key=%q", fixture.Version, fixture.MetadataKey)
	}
	for _, testCase := range fixture.Valid {
		t.Run("valid/"+testCase.Name, func(t *testing.T) {
			provenance, present, err := DecodeMetadata(testCase.Metadata, amber.IncomingReject)
			if err != nil || !present || provenance.WorkID() == "" {
				t.Fatalf("valid metadata rejected: present=%v err=%v", present, err)
			}
		})
	}
	for _, testCase := range fixture.Invalid {
		t.Run("invalid/"+testCase.Name, func(t *testing.T) {
			_, present, err := DecodeMetadata(testCase.Metadata, testCase.Policy)
			if testCase.ExpectError {
				if err == nil || present {
					t.Fatalf("expected metadata error: present=%v err=%v", present, err)
				}
				return
			}
			if err != nil || present != testCase.ExpectedPresent {
				t.Fatalf("unexpected result: present=%v err=%v", present, err)
			}
		})
	}
}
