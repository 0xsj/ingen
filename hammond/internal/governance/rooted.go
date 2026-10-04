package governance

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// VerifiedArtifact identifies exact bytes loaded through one explicit project
// root. Path is normalized and relative to that root.
type VerifiedArtifact struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// ApprovedVerification is a read-only verification result. Hammond validates
// its record with the existing replay policy and returns the governed identity
// plus the exact local artifacts consumed by verification.
type ApprovedVerification struct {
	Record    Record             `json:"record"`
	Contract  ContractReference  `json:"contract"`
	Policy    PolicyReference    `json:"policy"`
	Artifacts []VerifiedArtifact `json:"artifacts"`
}

// VerifyApprovedRooted verifies a Hammond approval record using only artifacts
// resolved under root. activePolicyPath is an explicit policy selection; the
// record's policy identity and digest must match those exact active bytes.
// expected is the contract identity the caller intends to use.
func VerifyApprovedRooted(root, approvalPath, activePolicyPath string, expected ContractReference) (ApprovedVerification, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return ApprovedVerification{}, fmt.Errorf("resolve Hammond project root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return ApprovedVerification{}, fmt.Errorf("inspect Hammond project root: %w", err)
	}
	if !info.IsDir() {
		return ApprovedVerification{}, fmt.Errorf("Hammond project root is not a directory")
	}

	approvalBytes, normalizedApprovalPath, err := readRooted(root, approvalPath)
	if err != nil {
		return ApprovedVerification{}, fmt.Errorf("read Hammond approval record: %w", err)
	}
	var record Record
	if err := decodeStrict(approvalBytes, &record); err != nil {
		return ApprovedVerification{}, fmt.Errorf("decode Hammond approval record: %w", err)
	}
	if record.State != StateApproved {
		return ApprovedVerification{}, fmt.Errorf("Hammond contract record state is %q, want approved", record.State)
	}
	if record.Contract.Identity() != expected.Identity() {
		return ApprovedVerification{}, fmt.Errorf("Hammond approval contract identity does not match the expected project/contract/version/schema/digest")
	}
	contractBytes, normalizedContractPath, err := readRooted(root, record.Contract.Artifact.URI)
	if err != nil {
		return ApprovedVerification{}, fmt.Errorf("read Hammond contract artifact: %w", err)
	}
	contractSHA := sha256.Sum256(contractBytes)
	contractDigest := hex.EncodeToString(contractSHA[:])
	if contractDigest != record.Contract.Artifact.SHA256 || contractDigest != expected.Artifact.SHA256 {
		return ApprovedVerification{}, fmt.Errorf("Hammond contract artifact bytes do not match the approved and expected digest")
	}

	activePolicyBytes, normalizedPolicyPath, err := readRooted(root, activePolicyPath)
	if err != nil {
		return ApprovedVerification{}, fmt.Errorf("read explicitly selected Hammond review policy: %w", err)
	}
	policySHA := sha256.Sum256(activePolicyBytes)
	policyDigest := hex.EncodeToString(policySHA[:])
	if policyDigest != record.Policy.Artifact.SHA256 {
		return ApprovedVerification{}, fmt.Errorf("active Hammond review policy bytes do not match the policy identity/hash recorded by the approval")
	}
	referencedPolicyBytes, normalizedReferencedPolicyPath, err := readRooted(root, record.Policy.Artifact.URI)
	if err != nil {
		return ApprovedVerification{}, fmt.Errorf("read Hammond review policy referenced by approval: %w", err)
	}
	if hashBytes(referencedPolicyBytes) != policyDigest {
		return ApprovedVerification{}, fmt.Errorf("Hammond review policy reference bytes do not match the explicitly selected active policy")
	}
	activePolicyReference := record.Policy
	activePolicyReference.Artifact.URI = normalizedPolicyPath
	authorityBytesByPath := make(map[string][]byte)
	policy, err := DecodeReviewPolicyWithAuthorityResolver(activePolicyBytes, activePolicyReference, func(reference AuthorityReference) (ReviewAuthority, error) {
		data, normalizedPath, readErr := readRooted(root, reference.Artifact.URI)
		if readErr != nil {
			return ReviewAuthority{}, fmt.Errorf("read Hammond review authority: %w", readErr)
		}
		authority, decodeErr := DecodeReviewAuthority(data, reference)
		if decodeErr != nil {
			return ReviewAuthority{}, decodeErr
		}
		authorityBytesByPath[normalizedPath] = append([]byte(nil), data...)
		return authority, nil
	})
	if err != nil {
		return ApprovedVerification{}, fmt.Errorf("validate active Hammond review policy and authority: %w", err)
	}
	if err := record.ValidateWithPolicy(policy); err != nil {
		return ApprovedVerification{}, fmt.Errorf("validate Hammond approval lifecycle: %w", err)
	}

	artifacts := []VerifiedArtifact{
		{Kind: "hammond-approval", Path: normalizedApprovalPath, SHA256: hashBytes(approvalBytes)},
		{Kind: "sorna-contract", Path: normalizedContractPath, SHA256: contractDigest},
		{Kind: "hammond-review-policy", Path: normalizedReferencedPolicyPath, SHA256: policyDigest},
	}
	if normalizedPolicyPath != normalizedReferencedPolicyPath {
		artifacts = append(artifacts, VerifiedArtifact{Kind: "active-hammond-review-policy", Path: normalizedPolicyPath, SHA256: policyDigest})
	}
	if !isEmptyAuthorityReference(policy.Authority) {
		normalizedAuthorityPath := filepath.ToSlash(filepath.Clean(policy.Authority.Artifact.URI))
		data, exists := authorityBytesByPath[normalizedAuthorityPath]
		if !exists {
			return ApprovedVerification{}, fmt.Errorf("Hammond review authority was not resolved during policy validation")
		}
		artifacts = append(artifacts, VerifiedArtifact{Kind: "hammond-review-authority", Path: normalizedAuthorityPath, SHA256: hashBytes(data)})
	}
	return ApprovedVerification{Record: record, Contract: record.Contract, Policy: activePolicyReference, Artifacts: artifacts}, nil
}

func readRooted(root, rawPath string) ([]byte, string, error) {
	if strings.TrimSpace(rawPath) == "" || strings.TrimSpace(rawPath) != rawPath {
		return nil, "", fmt.Errorf("artifact path is required and must not contain surrounding whitespace")
	}
	if strings.ContainsRune(rawPath, '\\') {
		return nil, "", fmt.Errorf("artifact path must use normalized slash-separated relative syntax")
	}
	parsed, err := url.Parse(rawPath)
	if err != nil {
		return nil, "", fmt.Errorf("artifact path is invalid: %w", err)
	}
	if parsed.Scheme != "" || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, "", fmt.Errorf("artifact reference must be a local project-relative path")
	}
	if filepath.IsAbs(rawPath) || filepath.VolumeName(rawPath) != "" {
		return nil, "", fmt.Errorf("artifact reference must be relative to the explicit project root")
	}
	clean := filepath.Clean(rawPath)
	if clean != rawPath || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, "", fmt.Errorf("artifact path %q is not normalized beneath the project root", rawPath)
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return nil, "", fmt.Errorf("open project root: %w", err)
	}
	defer rootHandle.Close()
	// O_NONBLOCK prevents an artifact swapped for a FIFO after validation
	// from hanging the verifier. The descriptor is then checked directly.
	file, err := rootHandle.OpenFile(clean, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() {
		return nil, "", fmt.Errorf("artifact path %q is not a regular file", rawPath)
	}
	contents, err := io.ReadAll(file)
	if err != nil {
		return nil, "", err
	}
	return contents, filepath.ToSlash(clean), nil
}

func hashBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
