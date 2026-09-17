package remote

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/custody"
	"ingen/lockwood/internal/store"
)

func TestReferenceCanonicalEncodingAndValidation(t *testing.T) {
	data := []byte("remote bytes")
	size := int64(len(data))
	reference := Reference{
		Schema:            Schema,
		URI:               "https://objects.example/runs/run-0001/result.json",
		Version:           "provider-version-0001",
		ETag:              `"strong-validator"`,
		ExpectedDigest:    artifact.DigestBytes(data),
		ExpectedSizeBytes: &size,
	}
	encoded, err := MarshalCanonical(reference)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := UnmarshalCanonical(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.URI != reference.URI || decoded.Version != reference.Version || decoded.ETag != reference.ETag || decoded.ExpectedDigest != reference.ExpectedDigest || decoded.ExpectedSizeBytes == nil || *decoded.ExpectedSizeBytes != size {
		t.Fatalf("decoded reference = %+v", decoded)
	}
	digest, err := CanonicalDigest(reference)
	if err != nil {
		t.Fatal(err)
	}
	if digest != artifact.DigestBytes(encoded) {
		t.Fatalf("canonical digest = %s, want digest of canonical bytes", digest)
	}
	if _, err := UnmarshalCanonical(append(encoded, '\n')); err == nil || !strings.Contains(err.Error(), "not canonical") {
		t.Fatalf("noncanonical reference error = %v", err)
	}
}

func TestReferenceRejectsUnsafeOrIncompleteValues(t *testing.T) {
	valid := Reference{Schema: Schema, URI: "https://objects.example/result.json", ExpectedDigest: artifact.DigestBytes([]byte("bytes"))}
	for _, test := range []struct {
		name   string
		modify func(*Reference)
		want   string
	}{
		{name: "credentials", modify: func(reference *Reference) { reference.URI = "https://user:secret@objects.example/result.json" }, want: "credential-free"},
		{name: "query", modify: func(reference *Reference) { reference.URI = "https://objects.example/result.json?token=secret" }, want: "query"},
		{name: "missing digest", modify: func(reference *Reference) { reference.ExpectedDigest = "" }, want: "expected digest"},
		{name: "negative size", modify: func(reference *Reference) { size := int64(-1); reference.ExpectedSizeBytes = &size }, want: "negative"},
		{name: "version whitespace", modify: func(reference *Reference) { reference.Version = "version 1" }, want: "whitespace"},
	} {
		t.Run(test.name, func(t *testing.T) {
			reference := valid
			test.modify(&reference)
			if err := reference.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestFetchAndVerifyChecksProviderPinsAndLocalIntegrity(t *testing.T) {
	data := []byte("remote bytes")
	size := int64(len(data))
	reference := Reference{
		Schema:            Schema,
		URI:               "https://objects.example/runs/run-0001/result.json",
		Version:           "version-0001",
		ETag:              `"etag-0001"`,
		ExpectedDigest:    artifact.DigestBytes(data),
		ExpectedSizeBytes: &size,
	}
	observedSize := int64(len(data))
	fetcher := &stubFetcher{result: FetchResult{
		Body:      io.NopCloser(bytes.NewReader(data)),
		URI:       reference.URI,
		Version:   reference.Version,
		ETag:      reference.ETag,
		SizeBytes: &observedSize,
	}}
	verified, err := FetchAndVerify(context.Background(), fetcher, reference, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(verified.Bytes, data) || verified.Digest != reference.ExpectedDigest || verified.SizeBytes != size || verified.Observed.ETag != reference.ETag || fetcher.calls != 1 {
		t.Fatalf("verified object = %+v, calls = %d", verified, fetcher.calls)
	}
}

func TestVerifiedRemoteObjectCanBeHandedToV2Custody(t *testing.T) {
	data := []byte("remote custody bytes")
	reference := Reference{
		Schema:         Schema,
		URI:            "https://objects.example/runs/run-0001/result.json",
		Version:        "version-0001",
		ExpectedDigest: artifact.DigestBytes(data),
	}
	verified, err := FetchAndVerify(context.Background(), &stubFetcher{result: FetchResult{
		Body:    io.NopCloser(bytes.NewReader(data)),
		URI:     reference.URI,
		Version: reference.Version,
	}}, reference, 0)
	if err != nil {
		t.Fatal(err)
	}
	artifacts := store.NewMemory()
	records := custody.NewMemory()
	ingestor, err := custody.NewIngestor(artifacts, records)
	if err != nil {
		t.Fatal(err)
	}
	record, err := ingestor.Accept(bytes.NewReader(verified.Bytes), custody.IntakeRequest{
		Schema:      custody.SchemaV2,
		CustodyID:   "lockwood-remote-custody-0001",
		MediaType:   "application/json",
		LogicalName: "remote-result.json",
		Producer:    custody.Producer{Tool: "remote-adapter", Kind: "object-fetch"},
		Source:      custody.Source{URI: verified.Observed.URI, Version: verified.Observed.Version},
		Handling:    custody.Handling{Redaction: "none", RetentionClass: "default"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Schema != custody.SchemaV2 || record.Source.URI != reference.URI || record.Source.Version != reference.Version || record.Artifact.Digest != reference.ExpectedDigest {
		t.Fatalf("remote custody record = %+v", record)
	}
	if _, err := custody.VerifyRecord(records, artifacts, record.CustodyID); err != nil {
		t.Fatal(err)
	}
}

func TestFetchAndVerifyRejectsPinBeforeReadingBody(t *testing.T) {
	data := &trackingBody{}
	reference := Reference{
		Schema:         Schema,
		URI:            "https://objects.example/result.json",
		ExpectedDigest: artifact.DigestBytes([]byte("remote bytes")),
	}
	_, err := FetchAndVerify(context.Background(), &stubFetcher{result: FetchResult{
		Body: data,
		URI:  "https://objects.example/replaced.json",
	}}, reference, 0)
	var failure *Failure
	if !errors.As(err, &failure) || failure.Class != PinMismatchFailure {
		t.Fatalf("error = %v, class = %v, want pin mismatch", err, failureClass(err))
	}
	if data.read || !data.closed {
		t.Fatalf("pin mismatch body state = read:%t closed:%t, want read:false closed:true", data.read, data.closed)
	}
}

func TestFetchAndVerifyClassifiesFailuresBeforeTrustedUse(t *testing.T) {
	data := []byte("remote bytes")
	base := Reference{Schema: Schema, URI: "https://objects.example/result.json", Version: "version-0001", ETag: `"etag-0001"`, ExpectedDigest: artifact.DigestBytes(data)}
	for _, test := range []struct {
		name   string
		ref    Reference
		result FetchResult
		err    error
		limit  int64
		class  FailureClass
	}{
		{name: "provider failure", ref: base, err: errors.New("provider unavailable"), class: ProviderFailure},
		{name: "uri pin mismatch", ref: base, result: FetchResult{Body: io.NopCloser(bytes.NewReader(data)), URI: "https://objects.example/other.json", Version: base.Version, ETag: base.ETag}, class: PinMismatchFailure},
		{name: "version pin mismatch", ref: base, result: FetchResult{Body: io.NopCloser(bytes.NewReader(data)), URI: base.URI, Version: "version-0002", ETag: base.ETag}, class: PinMismatchFailure},
		{name: "etag pin mismatch", ref: base, result: FetchResult{Body: io.NopCloser(bytes.NewReader(data)), URI: base.URI, Version: base.Version, ETag: `"etag-0002"`}, class: PinMismatchFailure},
		{name: "digest mismatch", ref: func() Reference {
			reference := base
			reference.ExpectedDigest = artifact.DigestBytes([]byte("other"))
			return reference
		}(), result: FetchResult{Body: io.NopCloser(bytes.NewReader(data)), URI: base.URI, Version: base.Version, ETag: base.ETag}, class: DigestMismatchFailure},
		{name: "size mismatch", ref: func() Reference {
			reference := base
			size := int64(len(data) + 1)
			reference.ExpectedSizeBytes = &size
			return reference
		}(), result: FetchResult{Body: io.NopCloser(bytes.NewReader(data)), URI: base.URI, Version: base.Version, ETag: base.ETag}, class: SizeMismatchFailure},
		{name: "maximum size", ref: base, result: FetchResult{Body: io.NopCloser(bytes.NewReader(data)), URI: base.URI, Version: base.Version, ETag: base.ETag}, limit: int64(len(data) - 1), class: SizeMismatchFailure},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := FetchAndVerify(context.Background(), &stubFetcher{result: test.result, err: test.err}, test.ref, test.limit)
			var failure *Failure
			if !errors.As(err, &failure) || failure.Class != test.class {
				t.Fatalf("error = %v, class = %v, want %v", err, failureClass(err), test.class)
			}
		})
	}
}

type stubFetcher struct {
	result FetchResult
	err    error
	calls  int
}

type trackingBody struct {
	read   bool
	closed bool
}

func (body *trackingBody) Read([]byte) (int, error) {
	body.read = true
	return 0, errors.New("body should not have been read")
}

func (body *trackingBody) Close() error {
	body.closed = true
	return nil
}

func (fetcher *stubFetcher) Fetch(_ context.Context, _ Reference) (FetchResult, error) {
	fetcher.calls++
	return fetcher.result, fetcher.err
}

func failureClass(err error) FailureClass {
	var failure *Failure
	if errors.As(err, &failure) {
		return failure.Class
	}
	return ""
}
