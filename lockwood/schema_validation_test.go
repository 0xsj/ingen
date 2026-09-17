package lockwood

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

const (
	artifactSchemaURL                    = "https://ingen.example/spec/lockwood.artifact-v1.schema.json"
	cleanupPlanSchemaURL                 = "https://ingen.example/spec/lockwood.cleanup-plan-v1.schema.json"
	cleanupAuthorizationTargetSchemaURL  = "https://ingen.example/spec/lockwood.cleanup-authorization-target-v1.schema.json"
	cleanupAuthorizationRequestSchemaURL = "https://ingen.example/spec/lockwood.cleanup-authorization-request-v1.schema.json"
	cleanupAuthorizationResultSchemaURL  = "https://ingen.example/spec/lockwood.cleanup-authorization-result-v1.schema.json"
	cleanupPolicyReferenceSchemaURL      = "https://ingen.example/spec/lockwood.cleanup-policy-reference-v1.schema.json"
	cleanupHoldReferenceSchemaURL        = "https://ingen.example/spec/lockwood.cleanup-hold-reference-v1.schema.json"
	cleanupReadinessSchemaURL            = "https://ingen.example/spec/lockwood.cleanup-readiness-v1.schema.json"
	cleanupRevalidationSchemaURL         = "https://ingen.example/spec/lockwood.cleanup-revalidation-v1.schema.json"
	cleanupLeaseSchemaURL                = "https://ingen.example/spec/lockwood.cleanup-lease-v1.schema.json"
	cleanupWorkerPreflightSchemaURL      = "https://ingen.example/spec/lockwood.cleanup-worker-preflight-v1.schema.json"
	cleanupOutcomeReceiptSchemaURL       = "https://ingen.example/spec/lockwood.cleanup-outcome-receipt-v1.schema.json"
	custodySchemaURL                     = "https://ingen.example/spec/lockwood.custody-v1.schema.json"
	custodyV2SchemaURL                   = "https://ingen.example/spec/lockwood.custody-v2.schema.json"
	attestationSchemaURL                 = "https://ingen.example/spec/lockwood.attestation-v1.schema.json"
	trustSchemaURL                       = "https://ingen.example/spec/lockwood.attestation-trust-v1.schema.json"
	handlingEventSchemaURL               = "https://ingen.example/spec/lockwood.handling-event-v1.schema.json"
	handlingEventAttestationSchemaURL    = "https://ingen.example/spec/lockwood.handling-event-attestation-v1.schema.json"
	handlingEventPolicySchemaURL         = "https://ingen.example/spec/lockwood.handling-event-policy-v1.schema.json"
	redactionProvenanceSchemaURL         = "https://ingen.example/spec/lockwood.redaction-provenance-attestation-v1.schema.json"
	remoteObjectSchemaURL                = "https://ingen.example/spec/lockwood.remote-object-v1.schema.json"
	verificationReportSchemaURL          = "https://ingen.example/spec/lockwood.verification-report-v1.schema.json"
)

func TestDraftSchemasValidateFixtures(t *testing.T) {
	custodySchema := compileLockwoodSchema(t, custodySchemaURL, "spec/lockwood.custody-v1.schema.json")
	artifactSchema := compileLockwoodSchema(t, artifactSchemaURL, "spec/lockwood.artifact-v1.schema.json")

	valid := loadJSONDocument(t, "testdata/valid-custody-record.json")
	if err := custodySchema.Validate(valid); err != nil {
		t.Fatalf("valid custody fixture rejected: %v", err)
	}
	artifact := valid.(map[string]any)["artifact"]
	if err := artifactSchema.Validate(artifact); err != nil {
		t.Fatalf("valid artifact reference rejected: %v", err)
	}
}

func TestDraftCustodySchemaRejectsInvalidFixtures(t *testing.T) {
	schema := compileLockwoodSchema(t, custodySchemaURL, "spec/lockwood.custody-v1.schema.json")
	for _, name := range []string{
		"invalid-digest.json",
		"invalid-missing-required.json",
		"invalid-lineage.json",
	} {
		t.Run(name, func(t *testing.T) {
			if err := schema.Validate(loadJSONDocument(t, filepath.Join("testdata", name))); err == nil {
				t.Fatal("invalid custody fixture was accepted by the JSON Schema")
			}
		})
	}
}

func TestDraftCustodyV2SchemaValidatesRemoteFixture(t *testing.T) {
	schema := compileLockwoodSchema(t, custodyV2SchemaURL, "spec/lockwood.custody-v2.schema.json")
	if err := schema.Validate(loadJSONDocument(t, "testdata/valid-remote-custody-v2-record.json")); err != nil {
		t.Fatalf("valid remote v2 custody fixture rejected: %v", err)
	}
}

func TestDraftAttestationSchemaValidatesFixture(t *testing.T) {
	schema := compileLockwoodSchema(t, attestationSchemaURL, "spec/lockwood.attestation-v1.schema.json")
	if err := schema.Validate(loadJSONDocument(t, "testdata/valid-attestation-v1.json")); err != nil {
		t.Fatalf("valid attestation rejected by draft schema: %v", err)
	}
}

func TestDraftAttestationTrustSchemaValidatesFixture(t *testing.T) {
	schema := compileLockwoodSchema(t, trustSchemaURL, "spec/lockwood.attestation-trust-v1.schema.json")
	if err := schema.Validate(loadJSONDocument(t, "testdata/valid-attestation-trust-v1.json")); err != nil {
		t.Fatalf("valid attestation trust registry rejected by draft schema: %v", err)
	}
}

func TestDraftHandlingEventSchemaValidatesFixture(t *testing.T) {
	schema := compileLockwoodSchema(t, handlingEventSchemaURL, "spec/lockwood.handling-event-v1.schema.json")
	if err := schema.Validate(loadJSONDocument(t, "testdata/valid-handling-event-v1.json")); err != nil {
		t.Fatalf("valid handling event rejected by draft schema: %v", err)
	}
}

func TestDraftHandlingEventAttestationSchemaValidatesFixture(t *testing.T) {
	schema := compileLockwoodSchema(t, handlingEventAttestationSchemaURL, "spec/lockwood.handling-event-attestation-v1.schema.json")
	if err := schema.Validate(loadJSONDocument(t, "testdata/valid-handling-event-attestation-v1.json")); err != nil {
		t.Fatalf("valid handling event attestation rejected by draft schema: %v", err)
	}
}

func TestDraftHandlingEventPolicySchemaValidatesFixture(t *testing.T) {
	schema := compileLockwoodSchema(t, handlingEventPolicySchemaURL, "spec/lockwood.handling-event-policy-v1.schema.json")
	if err := schema.Validate(loadJSONDocument(t, "testdata/valid-handling-event-policy-v1.json")); err != nil {
		t.Fatalf("valid handling event policy rejected by draft schema: %v", err)
	}
}

func TestDraftRedactionProvenanceSchemaValidatesFixture(t *testing.T) {
	schema := compileLockwoodSchema(t, redactionProvenanceSchemaURL, "spec/lockwood.redaction-provenance-attestation-v1.schema.json")
	if err := schema.Validate(loadJSONDocument(t, "testdata/valid-redaction-provenance-attestation-v1.json")); err != nil {
		t.Fatalf("valid redaction provenance attestation rejected by draft schema: %v", err)
	}
}

func TestDraftVerificationReportSchemaValidatesFixture(t *testing.T) {
	schema := compileLockwoodSchema(t, verificationReportSchemaURL, "spec/lockwood.verification-report-v1.schema.json")
	if err := schema.Validate(loadJSONDocument(t, "testdata/valid-verification-report-v1.json")); err != nil {
		t.Fatalf("valid verification report rejected by draft schema: %v", err)
	}
}

func TestDraftRemoteObjectSchemaValidatesFixture(t *testing.T) {
	schema := compileLockwoodSchema(t, remoteObjectSchemaURL, "spec/lockwood.remote-object-v1.schema.json")
	if err := schema.Validate(loadJSONDocument(t, "testdata/valid-remote-object-v1.json")); err != nil {
		t.Fatalf("valid remote object reference rejected by draft schema: %v", err)
	}
}

func TestDraftCleanupPlanSchemaValidatesFixture(t *testing.T) {
	schema := compileLockwoodSchema(t, cleanupPlanSchemaURL, "spec/lockwood.cleanup-plan-v1.schema.json")
	if err := schema.Validate(loadJSONDocument(t, "testdata/valid-cleanup-plan-v1.json")); err != nil {
		t.Fatalf("valid cleanup plan rejected by draft schema: %v", err)
	}
}

func TestDraftCleanupAuthorizationTargetSchemaValidatesFixture(t *testing.T) {
	schema := compileLockwoodSchema(t, cleanupAuthorizationTargetSchemaURL, "spec/lockwood.cleanup-authorization-target-v1.schema.json")
	if err := schema.Validate(loadJSONDocument(t, "testdata/valid-cleanup-authorization-target-v1.json")); err != nil {
		t.Fatalf("valid cleanup authorization target rejected by draft schema: %v", err)
	}
}

func TestDraftCleanupAuthorizationRequestSchemaValidatesFixture(t *testing.T) {
	schema := compileLockwoodSchema(t, cleanupAuthorizationRequestSchemaURL, "spec/lockwood.cleanup-authorization-request-v1.schema.json")
	if err := schema.Validate(loadJSONDocument(t, "testdata/valid-cleanup-authorization-request-v1.json")); err != nil {
		t.Fatalf("valid cleanup authorization request rejected by draft schema: %v", err)
	}
}

func TestDraftCleanupPolicyAndHoldReferenceSchemasValidateFixtures(t *testing.T) {
	policySchema := compileLockwoodSchema(t, cleanupPolicyReferenceSchemaURL, "spec/lockwood.cleanup-policy-reference-v1.schema.json")
	if err := policySchema.Validate(loadJSONDocument(t, "testdata/valid-cleanup-policy-reference-v1.json")); err != nil {
		t.Fatalf("valid cleanup policy reference rejected by draft schema: %v", err)
	}
	holdSchema := compileLockwoodSchema(t, cleanupHoldReferenceSchemaURL, "spec/lockwood.cleanup-hold-reference-v1.schema.json")
	if err := holdSchema.Validate(loadJSONDocument(t, "testdata/valid-cleanup-hold-reference-v1.json")); err != nil {
		t.Fatalf("valid cleanup hold reference rejected by draft schema: %v", err)
	}
}

func TestDraftCleanupAuthorizationResultSchemaValidatesFixture(t *testing.T) {
	schema := compileLockwoodSchema(t, cleanupAuthorizationResultSchemaURL, "spec/lockwood.cleanup-authorization-result-v1.schema.json")
	if err := schema.Validate(loadJSONDocument(t, "testdata/valid-cleanup-authorization-result-v1.json")); err != nil {
		t.Fatalf("valid cleanup authorization result rejected by draft schema: %v", err)
	}
}

func TestDraftCleanupReadinessSchemaValidatesFixture(t *testing.T) {
	schema := compileLockwoodSchema(t, cleanupReadinessSchemaURL, "spec/lockwood.cleanup-readiness-v1.schema.json")
	if err := schema.Validate(loadJSONDocument(t, "testdata/valid-cleanup-readiness-v1.json")); err != nil {
		t.Fatalf("valid cleanup readiness report rejected by draft schema: %v", err)
	}
}

func TestDraftCleanupPreflightSchemasValidateFixtures(t *testing.T) {
	for _, test := range []struct {
		name   string
		schema string
		path   string
		data   string
	}{
		{name: "revalidation", schema: cleanupRevalidationSchemaURL, path: "spec/lockwood.cleanup-revalidation-v1.schema.json", data: "testdata/valid-cleanup-revalidation-v1.json"},
		{name: "lease", schema: cleanupLeaseSchemaURL, path: "spec/lockwood.cleanup-lease-v1.schema.json", data: "testdata/valid-cleanup-lease-v1.json"},
		{name: "worker preflight", schema: cleanupWorkerPreflightSchemaURL, path: "spec/lockwood.cleanup-worker-preflight-v1.schema.json", data: "testdata/valid-cleanup-worker-preflight-v1.json"},
	} {
		t.Run(test.name, func(t *testing.T) {
			schema := compileLockwoodSchema(t, test.schema, test.path)
			if err := schema.Validate(loadJSONDocument(t, test.data)); err != nil {
				t.Fatalf("valid cleanup %s fixture rejected by draft schema: %v", test.name, err)
			}
		})
	}
}

func TestDraftCleanupOutcomeReceiptSchemaValidatesFixture(t *testing.T) {
	schema := compileLockwoodSchema(t, cleanupOutcomeReceiptSchemaURL, "spec/lockwood.cleanup-outcome-receipt-v1.schema.json")
	if err := schema.Validate(loadJSONDocument(t, "testdata/valid-cleanup-outcome-receipt-v1.json")); err != nil {
		t.Fatalf("valid cleanup outcome receipt rejected by draft schema: %v", err)
	}
}

func compileLockwoodSchema(t *testing.T, url, relativePath string) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat = true
	for _, resource := range []struct {
		url  string
		path string
	}{
		{url: artifactSchemaURL, path: "spec/lockwood.artifact-v1.schema.json"},
		{url: cleanupPlanSchemaURL, path: "spec/lockwood.cleanup-plan-v1.schema.json"},
		{url: cleanupAuthorizationTargetSchemaURL, path: "spec/lockwood.cleanup-authorization-target-v1.schema.json"},
		{url: cleanupAuthorizationRequestSchemaURL, path: "spec/lockwood.cleanup-authorization-request-v1.schema.json"},
		{url: cleanupAuthorizationResultSchemaURL, path: "spec/lockwood.cleanup-authorization-result-v1.schema.json"},
		{url: cleanupPolicyReferenceSchemaURL, path: "spec/lockwood.cleanup-policy-reference-v1.schema.json"},
		{url: cleanupHoldReferenceSchemaURL, path: "spec/lockwood.cleanup-hold-reference-v1.schema.json"},
		{url: cleanupReadinessSchemaURL, path: "spec/lockwood.cleanup-readiness-v1.schema.json"},
		{url: cleanupRevalidationSchemaURL, path: "spec/lockwood.cleanup-revalidation-v1.schema.json"},
		{url: cleanupLeaseSchemaURL, path: "spec/lockwood.cleanup-lease-v1.schema.json"},
		{url: cleanupWorkerPreflightSchemaURL, path: "spec/lockwood.cleanup-worker-preflight-v1.schema.json"},
		{url: cleanupOutcomeReceiptSchemaURL, path: "spec/lockwood.cleanup-outcome-receipt-v1.schema.json"},
		{url: custodySchemaURL, path: "spec/lockwood.custody-v1.schema.json"},
		{url: custodyV2SchemaURL, path: "spec/lockwood.custody-v2.schema.json"},
		{url: attestationSchemaURL, path: "spec/lockwood.attestation-v1.schema.json"},
		{url: trustSchemaURL, path: "spec/lockwood.attestation-trust-v1.schema.json"},
		{url: handlingEventSchemaURL, path: "spec/lockwood.handling-event-v1.schema.json"},
		{url: handlingEventAttestationSchemaURL, path: "spec/lockwood.handling-event-attestation-v1.schema.json"},
		{url: handlingEventPolicySchemaURL, path: "spec/lockwood.handling-event-policy-v1.schema.json"},
		{url: redactionProvenanceSchemaURL, path: "spec/lockwood.redaction-provenance-attestation-v1.schema.json"},
		{url: remoteObjectSchemaURL, path: "spec/lockwood.remote-object-v1.schema.json"},
		{url: verificationReportSchemaURL, path: "spec/lockwood.verification-report-v1.schema.json"},
	} {
		data, err := os.ReadFile(resource.path)
		if err != nil {
			t.Fatal(err)
		}
		if err := compiler.AddResource(resource.url, bytes.NewReader(data)); err != nil {
			t.Fatalf("add schema resource %s: %v", resource.url, err)
		}
	}
	schema, err := compiler.Compile(url)
	if err != nil {
		t.Fatalf("compile schema %s (%s): %v", url, relativePath, err)
	}
	return schema
}

func loadJSONDocument(t *testing.T, path string) any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return document
}
