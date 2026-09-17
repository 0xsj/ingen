package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"unicode"

	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/integrity"
)

const Schema = "lockwood.remote-object/v1"

// Reference identifies the provider object a caller intends to retrieve. It
// is descriptive locator metadata; ExpectedDigest remains the local byte
// identity required before a caller can treat the result as trusted custody.
type Reference struct {
	Schema            string `json:"schema"`
	URI               string `json:"uri"`
	Version           string `json:"version,omitempty"`
	ETag              string `json:"etag,omitempty"`
	ExpectedDigest    string `json:"expected_digest"`
	ExpectedSizeBytes *int64 `json:"expected_size_bytes,omitempty"`
}

func (reference Reference) Validate() error {
	if reference.Schema != Schema {
		return fmt.Errorf("unexpected remote object schema %q", reference.Schema)
	}
	parsed, err := url.Parse(reference.URI)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("remote object uri must be an absolute credential-free uri")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("remote object uri must not contain query or fragment credentials")
	}
	if err := validateOpaque("remote object version", reference.Version); err != nil {
		return err
	}
	if err := validateOpaque("remote object etag", reference.ETag); err != nil {
		return err
	}
	if err := artifact.ValidateDigest(reference.ExpectedDigest); err != nil {
		return fmt.Errorf("remote object expected digest: %w", err)
	}
	if reference.ExpectedSizeBytes != nil && *reference.ExpectedSizeBytes < 0 {
		return fmt.Errorf("remote object expected size cannot be negative")
	}
	return nil
}

func validateOpaque(name, value string) error {
	if value == "" {
		return nil
	}
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must not be blank", name)
	}
	for _, character := range value {
		if unicode.IsSpace(character) || unicode.IsControl(character) {
			return fmt.Errorf("%s must not contain whitespace or control characters", name)
		}
	}
	return nil
}

func MarshalCanonical(reference Reference) ([]byte, error) {
	if err := reference.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(reference)
}

func UnmarshalCanonical(data []byte) (Reference, error) {
	var reference Reference
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&reference); err != nil {
		return Reference{}, fmt.Errorf("decode remote object reference: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Reference{}, fmt.Errorf("remote object reference contains multiple JSON values")
		}
		return Reference{}, fmt.Errorf("decode remote object reference: %w", err)
	}
	canonical, err := MarshalCanonical(reference)
	if err != nil {
		return Reference{}, err
	}
	if !bytes.Equal(data, canonical) {
		return Reference{}, fmt.Errorf("remote object reference is not canonical JSON")
	}
	return reference, nil
}

func CanonicalDigest(reference Reference) (string, error) {
	data, err := MarshalCanonical(reference)
	if err != nil {
		return "", err
	}
	return artifact.DigestBytes(data), nil
}

// Fetcher is the caller-owned provider boundary. It owns credentials,
// authentication, retries, rate limits, and cancellation with the provider.
type Fetcher interface {
	Fetch(context.Context, Reference) (FetchResult, error)
}

// FetchResult is one finite provider response. The provider adapter must not
// return credentials in these fields or in the reference it was given.
type FetchResult struct {
	Body      io.ReadCloser
	URI       string
	Version   string
	ETag      string
	SizeBytes *int64
}

type Observed struct {
	URI       string
	Version   string
	ETag      string
	SizeBytes int64
}

type VerifiedObject struct {
	Reference Reference
	Observed  Observed
	Bytes     []byte
	Digest    string
	SizeBytes int64
}

type FailureClass string

const (
	InvalidReferenceFailure FailureClass = "invalid-reference"
	ProviderFailure         FailureClass = "provider-failure"
	PinMismatchFailure      FailureClass = "pin-mismatch"
	StreamFailure           FailureClass = "stream-failure"
	DigestMismatchFailure   FailureClass = "digest-mismatch"
	SizeMismatchFailure     FailureClass = "size-mismatch"
)

type Failure struct {
	Class FailureClass
	Err   error
}

func (failure *Failure) Error() string {
	return fmt.Sprintf("remote object %s: %v", failure.Class, failure.Err)
}

func (failure *Failure) Unwrap() error {
	return failure.Err
}

// FetchAndVerify performs no local publication. It validates the reference,
// asks the caller-owned fetcher for one response, checks provider pins and
// observed size, and verifies the complete byte stream against local digest
// and size expectations. The returned bytes may then be passed to custody.
func FetchAndVerify(ctx context.Context, fetcher Fetcher, reference Reference, maxBytes int64) (VerifiedObject, error) {
	if err := reference.Validate(); err != nil {
		return VerifiedObject{}, &Failure{Class: InvalidReferenceFailure, Err: err}
	}
	if fetcher == nil {
		return VerifiedObject{}, &Failure{Class: ProviderFailure, Err: fmt.Errorf("remote fetcher is required")}
	}
	if maxBytes < 0 {
		return VerifiedObject{}, &Failure{Class: SizeMismatchFailure, Err: fmt.Errorf("maximum artifact size cannot be negative")}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return VerifiedObject{}, &Failure{Class: ProviderFailure, Err: ctx.Err()}
	default:
	}

	result, err := fetcher.Fetch(ctx, reference)
	if err != nil {
		return VerifiedObject{}, &Failure{Class: ProviderFailure, Err: err}
	}
	if result.Body == nil {
		return VerifiedObject{}, &Failure{Class: StreamFailure, Err: fmt.Errorf("remote fetch body is required")}
	}
	if result.URI != reference.URI {
		_ = result.Body.Close()
		return VerifiedObject{}, &Failure{Class: PinMismatchFailure, Err: fmt.Errorf("observed uri %q does not match requested uri %q", result.URI, reference.URI)}
	}
	if err := validateOpaque("observed remote object version", result.Version); err != nil {
		_ = result.Body.Close()
		return VerifiedObject{}, &Failure{Class: PinMismatchFailure, Err: err}
	}
	if err := validateOpaque("observed remote object etag", result.ETag); err != nil {
		_ = result.Body.Close()
		return VerifiedObject{}, &Failure{Class: PinMismatchFailure, Err: err}
	}
	if reference.Version != "" && result.Version != reference.Version {
		_ = result.Body.Close()
		return VerifiedObject{}, &Failure{Class: PinMismatchFailure, Err: fmt.Errorf("observed version %q does not match requested version %q", result.Version, reference.Version)}
	}
	if reference.ETag != "" && result.ETag != reference.ETag {
		_ = result.Body.Close()
		return VerifiedObject{}, &Failure{Class: PinMismatchFailure, Err: fmt.Errorf("observed etag %q does not match requested etag %q", result.ETag, reference.ETag)}
	}
	if result.SizeBytes != nil && *result.SizeBytes < 0 {
		_ = result.Body.Close()
		return VerifiedObject{}, &Failure{Class: SizeMismatchFailure, Err: fmt.Errorf("observed size cannot be negative")}
	}
	data, measured, readErr := integrity.ReadAll(result.Body, maxBytes)
	closeErr := result.Body.Close()
	if readErr != nil {
		return VerifiedObject{}, &Failure{Class: streamFailureClass(readErr), Err: readErr}
	}
	if closeErr != nil {
		return VerifiedObject{}, &Failure{Class: StreamFailure, Err: fmt.Errorf("close remote fetch body: %w", closeErr)}
	}

	if result.SizeBytes != nil && *result.SizeBytes != measured.SizeBytes {
		return VerifiedObject{}, &Failure{Class: SizeMismatchFailure, Err: fmt.Errorf("observed size %d does not match measured size %d", *result.SizeBytes, measured.SizeBytes)}
	}
	if reference.ExpectedSizeBytes != nil && *reference.ExpectedSizeBytes != measured.SizeBytes {
		return VerifiedObject{}, &Failure{Class: SizeMismatchFailure, Err: fmt.Errorf("expected size %d, measured %d", *reference.ExpectedSizeBytes, measured.SizeBytes)}
	}
	if measured.Digest != reference.ExpectedDigest {
		return VerifiedObject{}, &Failure{Class: DigestMismatchFailure, Err: fmt.Errorf("expected digest %s, computed %s", reference.ExpectedDigest, measured.Digest)}
	}

	return VerifiedObject{
		Reference: reference,
		Observed: Observed{
			URI:       result.URI,
			Version:   result.Version,
			ETag:      result.ETag,
			SizeBytes: measured.SizeBytes,
		},
		Bytes:     data,
		Digest:    measured.Digest,
		SizeBytes: measured.SizeBytes,
	}, nil
}

func streamFailureClass(err error) FailureClass {
	if strings.Contains(err.Error(), "exceeds maximum size") {
		return SizeMismatchFailure
	}
	return StreamFailure
}
