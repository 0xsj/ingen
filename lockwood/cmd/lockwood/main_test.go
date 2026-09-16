package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"ingen/lockwood/internal/attestation"
	"ingen/lockwood/internal/custody"
)

func TestCLIEndToEnd(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(input, []byte(`{"schema":"ingen.ci-result/v1","status":"passed"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var putOutput bytes.Buffer
	if code := run([]string{
		"put",
		"--root", root,
		"--id", "lockwood-cli-0001",
		"--media-type", "application/json",
		"--producer", "sorna",
		"--kind", "behavioral-verification",
		"--run-id", "run-0001",
		input,
	}, strings.NewReader(""), &putOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("put exit code = %d", code)
	}
	var record map[string]any
	if err := json.Unmarshal(putOutput.Bytes(), &record); err != nil {
		t.Fatalf("decode put output: %v", err)
	}
	digest := record["artifact"].(map[string]any)["digest"].(string)

	var inspectOutput bytes.Buffer
	if code := run([]string{"inspect", "--root", root, "lockwood-cli-0001"}, strings.NewReader(""), &inspectOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("inspect exit code = %d", code)
	}
	if !strings.Contains(inspectOutput.String(), digest) {
		t.Fatalf("inspect output does not contain digest: %s", inspectOutput.String())
	}

	var recordDigestOutput bytes.Buffer
	if code := run([]string{"record-digest", "--root", root, "lockwood-cli-0001"}, strings.NewReader(""), &recordDigestOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("record-digest exit code = %d", code)
	}
	recordDigest := strings.TrimSpace(recordDigestOutput.String())
	if !strings.HasPrefix(recordDigest, "sha256:") || len(recordDigest) != len("sha256:")+64 {
		t.Fatalf("record digest = %q, want sha256 digest", recordDigest)
	}

	var findOutput bytes.Buffer
	if code := run([]string{"find", "--root", root, "--producer", "sorna"}, strings.NewReader(""), &findOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("find exit code = %d", code)
	}
	if !strings.Contains(findOutput.String(), "lockwood-cli-0001") {
		t.Fatalf("find output does not contain custody ID: %s", findOutput.String())
	}

	var getOutput bytes.Buffer
	if code := run([]string{"get", "--root", root, digest}, strings.NewReader(""), &getOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("get exit code = %d", code)
	}
	if getOutput.String() != `{"schema":"ingen.ci-result/v1","status":"passed"}` {
		t.Fatalf("get output = %q", getOutput.String())
	}

	var verifyOutput bytes.Buffer
	if code := run([]string{"verify", "--root", root, digest}, strings.NewReader(""), &verifyOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("verify exit code = %d", code)
	}
	if strings.TrimSpace(verifyOutput.String()) != digest {
		t.Fatalf("verify output = %q", verifyOutput.String())
	}

	var custodyVerifyOutput bytes.Buffer
	if code := run([]string{"verify", "--root", root, "--id", "lockwood-cli-0001"}, strings.NewReader(""), &custodyVerifyOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("custody verify exit code = %d", code)
	}
	if !strings.Contains(custodyVerifyOutput.String(), "lockwood-cli-0001") {
		t.Fatalf("custody verify output = %s", custodyVerifyOutput.String())
	}

	var reconcileOutput bytes.Buffer
	if code := run([]string{"reconcile", "--root", root}, strings.NewReader(""), &reconcileOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("reconcile exit code = %d", code)
	}
	var report struct {
		Orphans            []any `json:"orphans"`
		DanglingReferences []any `json:"dangling_references"`
		CorruptBlobs       []any `json:"corrupt_blobs"`
	}
	if err := json.Unmarshal(reconcileOutput.Bytes(), &report); err != nil {
		t.Fatalf("decode reconcile output: %v", err)
	}
	if len(report.Orphans) != 0 || len(report.DanglingReferences) != 0 || len(report.CorruptBlobs) != 0 {
		t.Fatalf("reconcile report = %+v, want clean root", report)
	}
}

func TestCLIGetRefusesOverwrite(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(input, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	var putOutput bytes.Buffer
	if code := run([]string{"put", "--root", root, "--id", "lockwood-cli-overwrite", "--media-type", "text/plain", "--producer", "example", "--kind", "fixture", input}, strings.NewReader(""), &putOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("put exit code = %d", code)
	}
	var record map[string]any
	if err := json.Unmarshal(putOutput.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	digest := record["artifact"].(map[string]any)["digest"].(string)
	output := filepath.Join(t.TempDir(), "output.txt")
	if err := os.WriteFile(output, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if code := run([]string{"get", "--root", root, "--output", output, digest}, strings.NewReader(""), &bytes.Buffer{}, &stderr); code == 0 {
		t.Fatal("get overwrote an existing output")
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "existing" {
		t.Fatalf("existing output changed to %q", data)
	}
}

func TestCLIPutRemoteV2Source(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "remote-result.json")
	if err := os.WriteFile(input, []byte("remote bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := run([]string{
		"put",
		"--root", root,
		"--schema", "lockwood.custody/v2",
		"--id", "lockwood-cli-remote-v2",
		"--media-type", "application/json",
		"--producer", "sorna",
		"--kind", "remote-result",
		"--source-uri", "s3://evidence.example/runs/run-0001/result.json",
		"--source-version", "version-0001",
		input,
	}, strings.NewReader(""), &output, &bytes.Buffer{}); code != 0 {
		t.Fatalf("put remote v2 exit code = %d", code)
	}
	var record struct {
		Schema string `json:"schema"`
		Source struct {
			URI     string `json:"uri"`
			Version string `json:"version"`
		} `json:"source"`
	}
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record.Schema != "lockwood.custody/v2" || record.Source.URI == "" || record.Source.Version == "" {
		t.Fatalf("remote v2 record = %+v", record)
	}
}

func TestCLIPutRejectsOversizedInput(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "oversized.txt")
	if err := os.WriteFile(input, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if code := run([]string{
		"put",
		"--root", root,
		"--id", "lockwood-cli-oversized",
		"--media-type", "text/plain",
		"--producer", "example",
		"--kind", "fixture",
		"--max-bytes", "3",
		input,
	}, strings.NewReader(""), &bytes.Buffer{}, &stderr); code == 0 {
		t.Fatal("put accepted an oversized input")
	}
	if !strings.Contains(stderr.String(), "exceeds maximum size") {
		t.Fatalf("stderr = %q, want maximum-size rejection", stderr.String())
	}
}

func TestCLIImportSorna(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(t.TempDir(), "sorna-run")
	if err := os.MkdirAll(bundle, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"manifest.json": []byte("{\"schema\":\"sorna.evidence/v1\",\"run_id\":\"run-cli-0001\"}\n"),
		"run.json":      []byte("{\"schema\":\"ingen.run/v1\",\"run_id\":\"run-cli-0001\"}\n"),
	}
	checksums := make([]string, 0, len(files))
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(bundle, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		checksums = append(checksums, hex.EncodeToString(digest[:])+"  "+name)
	}
	sort.Strings(checksums)
	if err := os.WriteFile(filepath.Join(bundle, "checksums.sha256"), []byte(strings.Join(checksums, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if code := run([]string{"import-sorna", "--root", root, "--id", "lockwood-cli-sorna-0001", bundle}, strings.NewReader(""), &output, &bytes.Buffer{}); code != 0 {
		t.Fatalf("import-sorna exit code = %d", code)
	}
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record["producer"].(map[string]any)["kind"] != "evidence-bundle" {
		t.Fatalf("producer = %+v", record["producer"])
	}
}

func TestCLIImportCIResult(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "ci-result.json")
	contents := []byte(`{"schema":"ingen.ci-result/v1","tool":"paddock","kind":"architecture","status":"passed","exit_code":0,"created_at":"2026-09-15T12:00:00Z","source":{"root":"/workspace/service"},"report":{},"explanation":{}}`)
	if err := os.WriteFile(input, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := run([]string{"import-ci-result", "--root", root, "--id", "lockwood-cli-ci-result-0001", input}, strings.NewReader(""), &output, &bytes.Buffer{}); code != 0 {
		t.Fatalf("import-ci-result exit code = %d", code)
	}
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record["status"] != "accepted" || record["producer"].(map[string]any)["tool"] != "paddock" {
		t.Fatalf("imported custody record = %+v", record)
	}
	if record["artifact"].(map[string]any)["media_type"] != "application/vnd.ingen.ci-result+json" {
		t.Fatalf("imported media type = %+v", record["artifact"])
	}
}

func TestCLIVerifyAttestation(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "attested.txt")
	if err := os.WriteFile(input, []byte("attested bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	const custodyID = "lockwood-cli-attestation-0001"
	var putOutput bytes.Buffer
	if code := run([]string{
		"put",
		"--root", root,
		"--id", custodyID,
		"--media-type", "text/plain",
		"--producer", "example",
		"--kind", "attestation-fixture",
		input,
	}, strings.NewReader(""), &putOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("put exit code = %d", code)
	}
	var record custody.Record
	if err := json.Unmarshal(putOutput.Bytes(), &record); err != nil {
		t.Fatalf("decode put output: %v", err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{18}, ed25519.SeedSize))
	envelope, err := attestation.Sign(record, "review-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	artifacts, _, err := openStores(root)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := attestation.Publish(envelope, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	targetDigest, err := custody.CanonicalDigest(record)
	if err != nil {
		t.Fatal(err)
	}
	var findOutput bytes.Buffer
	if code := run([]string{"find-attestation", "--root", root, "--target-digest", targetDigest}, strings.NewReader(""), &findOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("find-attestation exit code = %d", code)
	}
	var found []attestation.StoredAttestation
	if err := json.Unmarshal(findOutput.Bytes(), &found); err != nil {
		t.Fatalf("decode attestation find output: %v", err)
	}
	if len(found) != 1 || found[0].Artifact.Digest != ref.Digest || found[0].Envelope.KeyID != "review-key-2026-01" {
		t.Fatalf("attestation find results = %+v", found)
	}
	var inspectOutput bytes.Buffer
	if code := run([]string{"inspect-attestation", "--root", root, ref.Digest}, strings.NewReader(""), &inspectOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("inspect-attestation exit code = %d", code)
	}
	var inspection attestation.Inspection
	if err := json.Unmarshal(inspectOutput.Bytes(), &inspection); err != nil {
		t.Fatalf("decode attestation inspection: %v", err)
	}
	if inspection.AttestationDigest != ref.Digest || inspection.Envelope.KeyID != "review-key-2026-01" {
		t.Fatalf("attestation inspection = %+v", inspection)
	}
	publicKeyPath := filepath.Join(t.TempDir(), "review-key.pub")
	encodedKey := base64.StdEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey)) + "\n"
	if err := os.WriteFile(publicKeyPath, []byte(encodedKey), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := run([]string{
		"verify-attestation",
		"--root", root,
		"--id", custodyID,
		"--public-key", publicKeyPath,
		ref.Digest,
	}, strings.NewReader(""), &output, &bytes.Buffer{}); code != 0 {
		t.Fatalf("verify-attestation exit code = %d", code)
	}
	var receipt attestation.VerificationReceipt
	if err := json.Unmarshal(output.Bytes(), &receipt); err != nil {
		t.Fatalf("decode verification receipt: %v", err)
	}
	if receipt.CustodyID != custodyID || receipt.AttestationDigest != ref.Digest || receipt.KeyID != "review-key-2026-01" || receipt.Algorithm != attestation.Algorithm || !receipt.Verified {
		t.Fatalf("verification receipt = %+v", receipt)
	}

	w := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{19}, ed25519.SeedSize))
	if err := os.WriteFile(publicKeyPath, []byte(base64.StdEncoding.EncodeToString(w.Public().(ed25519.PublicKey))+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if code := run([]string{
		"verify-attestation",
		"--root", root,
		"--id", custodyID,
		"--public-key", publicKeyPath,
		ref.Digest,
	}, strings.NewReader(""), &bytes.Buffer{}, &stderr); code == 0 {
		t.Fatal("verify-attestation accepted a wrong public key")
	}
	if !strings.Contains(stderr.String(), "verification failed") {
		t.Fatalf("wrong-key stderr = %q", stderr.String())
	}
}

func TestCLISignAttestation(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "attested.txt")
	if err := os.WriteFile(input, []byte("attested bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	const custodyID = "lockwood-cli-sign-attestation-0001"
	var putOutput bytes.Buffer
	if code := run([]string{
		"put",
		"--root", root,
		"--id", custodyID,
		"--media-type", "text/plain",
		"--producer", "example",
		"--kind", "attestation-fixture",
		input,
	}, strings.NewReader(""), &putOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("put exit code = %d", code)
	}
	var record custody.Record
	if err := json.Unmarshal(putOutput.Bytes(), &record); err != nil {
		t.Fatalf("decode put output: %v", err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{20}, ed25519.SeedSize))
	privateKeyPath := filepath.Join(t.TempDir(), "review-key.key")
	if err := os.WriteFile(privateKeyPath, []byte(base64.StdEncoding.EncodeToString(privateKey)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var signOutput bytes.Buffer
	if code := run([]string{
		"sign-attestation",
		"--root", root,
		"--id", custodyID,
		"--key-id", "review-key-2026-01",
		"--private-key", privateKeyPath,
	}, strings.NewReader(""), &signOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("sign-attestation exit code = %d", code)
	}
	var result struct {
		CustodyID string `json:"custody_id"`
		Artifact  struct {
			Digest    string `json:"digest"`
			MediaType string `json:"media_type"`
		} `json:"artifact"`
		Envelope attestation.Envelope `json:"envelope"`
	}
	if err := json.Unmarshal(signOutput.Bytes(), &result); err != nil {
		t.Fatalf("decode sign-attestation output: %v", err)
	}
	if result.CustodyID != custodyID || result.Artifact.Digest == "" || result.Artifact.MediaType != attestation.MediaType {
		t.Fatalf("published artifact = %+v", result.Artifact)
	}
	if result.Envelope.KeyID != "review-key-2026-01" {
		t.Fatalf("published envelope = %+v", result.Envelope)
	}
	recordDigest, err := custody.CanonicalDigest(record)
	if err != nil {
		t.Fatal(err)
	}
	if result.Envelope.Target.Digest != recordDigest {
		t.Fatalf("target digest = %s, want %s", result.Envelope.Target.Digest, recordDigest)
	}
	artifacts, _, err := openStores(root)
	if err != nil {
		t.Fatal(err)
	}
	keys := attestation.PublicKeySet{"review-key-2026-01": privateKey.Public().(ed25519.PublicKey)}
	if err := attestation.VerifyPublished(record, result.Artifact.Digest, artifacts, keys); err != nil {
		t.Fatalf("published CLI attestation failed verification: %v", err)
	}

	if err := os.Chmod(privateKeyPath, 0o644); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if code := run([]string{
		"sign-attestation",
		"--root", root,
		"--id", custodyID,
		"--key-id", "review-key-2026-01",
		"--private-key", privateKeyPath,
	}, strings.NewReader(""), &bytes.Buffer{}, &stderr); code == 0 {
		t.Fatal("sign-attestation accepted a private key file with open permissions")
	}
	if !strings.Contains(stderr.String(), "permissions are too open") {
		t.Fatalf("open-permission stderr = %q", stderr.String())
	}
}

func TestCLIImportAttestation(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(t.TempDir(), "attested.txt")
	if err := os.WriteFile(input, []byte("attested bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	const custodyID = "lockwood-cli-import-attestation-0001"
	var putOutput bytes.Buffer
	if code := run([]string{
		"put",
		"--root", root,
		"--id", custodyID,
		"--media-type", "text/plain",
		"--producer", "example",
		"--kind", "attestation-fixture",
		input,
	}, strings.NewReader(""), &putOutput, &bytes.Buffer{}); code != 0 {
		t.Fatalf("put exit code = %d", code)
	}
	var record custody.Record
	if err := json.Unmarshal(putOutput.Bytes(), &record); err != nil {
		t.Fatalf("decode put output: %v", err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{22}, ed25519.SeedSize))
	envelope, err := attestation.Sign(record, "external-review-key-2026-01", privateKey)
	if err != nil {
		t.Fatal(err)
	}
	envelopePath := filepath.Join(t.TempDir(), "attestation.json")
	envelopeBytes, err := attestation.MarshalCanonical(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envelopePath, envelopeBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	envelopeDigest, err := attestation.CanonicalDigest(envelope)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := run([]string{
		"import-attestation",
		"--root", root,
		"--id", custodyID,
		"--expected-digest", envelopeDigest,
		envelopePath,
	}, strings.NewReader(""), &output, &bytes.Buffer{}); code != 0 {
		t.Fatalf("import-attestation exit code = %d", code)
	}
	var publication attestation.Publication
	if err := json.Unmarshal(output.Bytes(), &publication); err != nil {
		t.Fatalf("decode import-attestation output: %v", err)
	}
	if publication.CustodyID != custodyID || publication.Artifact.Digest != envelopeDigest {
		t.Fatalf("import publication = %+v", publication)
	}
	artifacts, _, err := openStores(root)
	if err != nil {
		t.Fatal(err)
	}
	keys := attestation.PublicKeySet{"external-review-key-2026-01": privateKey.Public().(ed25519.PublicKey)}
	if err := attestation.VerifyPublished(record, publication.Artifact.Digest, artifacts, keys); err != nil {
		t.Fatalf("imported attestation failed verification: %v", err)
	}

	var stderr bytes.Buffer
	if code := run([]string{
		"import-attestation",
		"--root", root,
		"--id", custodyID,
		"--expected-digest", "sha256:" + strings.Repeat("0", 64),
		envelopePath,
	}, strings.NewReader(""), &bytes.Buffer{}, &stderr); code == 0 {
		t.Fatal("import-attestation accepted a mismatched expected digest")
	}
	if !strings.Contains(stderr.String(), "expected digest") {
		t.Fatalf("mismatched-digest stderr = %q", stderr.String())
	}
}

func TestCLIRecover(t *testing.T) {
	root := t.TempDir()
	seedPath := filepath.Join(t.TempDir(), "seed.txt")
	orphanPath := filepath.Join(t.TempDir(), "orphan.txt")
	if err := os.WriteFile(seedPath, []byte("seed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphanPath, []byte("orphan"), 0o600); err != nil {
		t.Fatal(err)
	}
	const custodyID = "lockwood-cli-recover-0001"
	if code := run([]string{
		"put",
		"--root", root,
		"--id", custodyID,
		"--media-type", "text/plain",
		"--producer", "example",
		"--kind", "seed",
		seedPath,
	}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("seed put exit code = %d", code)
	}

	pendingPath := filepath.Join(t.TempDir(), "pending.json")
	var putStderr bytes.Buffer
	if code := run([]string{
		"put",
		"--root", root,
		"--id", custodyID,
		"--media-type", "text/plain",
		"--producer", "example",
		"--kind", "recovered",
		"--pending-record", pendingPath,
		orphanPath,
	}, strings.NewReader(""), &bytes.Buffer{}, &putStderr); code == 0 {
		t.Fatal("conflicting put unexpectedly succeeded")
	}
	if !strings.Contains(putStderr.String(), "pending record") {
		t.Fatalf("put stderr = %q, want pending-record confirmation", putStderr.String())
	}
	if err := os.Remove(filepath.Join(root, "records", custodyID+".json")); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if code := run([]string{"recover", "--root", root, pendingPath}, strings.NewReader(""), &output, &bytes.Buffer{}); code != 0 {
		t.Fatalf("recover exit code = %d", code)
	}
	if !strings.Contains(output.String(), `"custody_id": "`+custodyID+`"`) {
		t.Fatalf("recover output = %s", output.String())
	}
	if code := run([]string{"verify", "--root", root, "--id", custodyID}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatal("recovered custody record failed verification")
	}
}
