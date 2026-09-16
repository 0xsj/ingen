package integrity

import (
	"bytes"
	"strings"
	"testing"

	"ingen/lockwood/internal/artifact"
)

func TestCopyComputesDigestAndSize(t *testing.T) {
	contents := []byte("integrity input")
	var output bytes.Buffer
	result, err := Copy(&output, bytes.NewReader(contents), 0)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), contents) {
		t.Fatalf("copied output = %q, want %q", output.Bytes(), contents)
	}
	if result.Digest != artifact.DigestBytes(contents) || result.SizeBytes != int64(len(contents)) {
		t.Fatalf("integrity result = %+v", result)
	}
	if err := VerifyBytes(contents, result.Digest); err != nil {
		t.Fatal(err)
	}
}

func TestCopyRejectsOversizedInput(t *testing.T) {
	var output bytes.Buffer
	_, err := Copy(&output, strings.NewReader("1234"), 3)
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Fatalf("Copy error = %v, want maximum-size rejection", err)
	}
	if output.String() != "123" {
		t.Fatalf("bounded output = %q, want only the allowed prefix", output.String())
	}
}

func TestCopyRejectsNegativeLimitAndVerifyDigestMismatch(t *testing.T) {
	if _, err := Copy(&bytes.Buffer{}, strings.NewReader("data"), -1); err == nil || !strings.Contains(err.Error(), "cannot be negative") {
		t.Fatalf("negative-limit error = %v", err)
	}
	if err := VerifyBytes([]byte("data"), "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"); err == nil || !strings.Contains(err.Error(), "expected digest") {
		t.Fatalf("digest mismatch error = %v", err)
	}
}
