// Package evidence writes and verifies the first Sorna evidence bundle.
package evidence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ingen/sorna/internal/policy"
	"ingen/sorna/internal/runner"
)

// Bundle describes the files written for one run.
type Bundle struct {
	RootDir               string
	RunPath               string
	ManifestPath          string
	LifecyclePath         string
	SubjectAccessPath     string
	SubjectExecutablePath string
	ChecksumsPath         string
}

// Manifest is the bundle entrypoint. It records identity and hashes, but does
// not turn content integrity into a correctness or isolation claim.
type Manifest struct {
	Schema          string                    `json:"schema"`
	RunID           string                    `json:"run_id"`
	CreatedAt       time.Time                 `json:"created_at"`
	Assurance       runner.Assurance          `json:"assurance"`
	Contract        runner.ContractReference  `json:"contract"`
	Oracle          *runner.OracleReference   `json:"oracle,omitempty"`
	Baseline        *runner.BaselineReference `json:"baseline,omitempty"`
	Subject         runner.SubjectReference   `json:"subject"`
	Policy          *policy.Reference         `json:"policy,omitempty"`
	SubjectPolicy   *policy.Reference         `json:"subject_policy,omitempty"`
	Campaign        *CampaignProvenance       `json:"campaign,omitempty"`
	ArtifactsSHA256 map[string]string         `json:"artifacts_sha256"`
}

// CampaignProvenance records the exact campaign inputs used to produce one
// per-mutation evidence bundle. The raw inputs are copied into the bundle so
// later source-file changes cannot silently change what the evidence means.
type CampaignProvenance struct {
	Schema     string                    `json:"schema"`
	RunID      string                    `json:"run_id"`
	Sequence   int                       `json:"sequence"`
	MutationID string                    `json:"mutation_id"`
	Plan       CampaignArtifactReference `json:"plan"`
	Provider   CampaignArtifactReference `json:"provider"`
}

// CampaignArtifactReference binds a copied campaign input to the source path
// supplied to the campaign executor and to its exact byte hash.
type CampaignArtifactReference struct {
	SourcePath string `json:"source_path"`
	BundlePath string `json:"bundle_path"`
	SHA256     string `json:"sha256"`
}

// CampaignProvenanceInput identifies the campaign inputs to copy into one
// mutation evidence bundle. When bytes are provided, they are the exact
// captured inputs that the executor parsed; otherwise the paths are read at
// attachment time. Files are copied byte-for-byte, without re-serializing.
type CampaignProvenanceInput struct {
	PlanPath      string
	PlanBytes     []byte
	ProviderPath  string
	ProviderBytes []byte
	Sequence      int
	MutationID    string
}

const campaignProvenanceSchema = "sorna.campaign-provenance/v1"

// LifecycleEvent is the append-only JSONL form of a lifecycle record event.
// The event envelope gives each line run identity and an actor without making
// the lower-level lifecycle package depend on evidence storage.
type LifecycleEvent struct {
	EventID   string         `json:"event_id"`
	RunID     string         `json:"run_id"`
	Sequence  int            `json:"sequence"`
	Timestamp time.Time      `json:"timestamp"`
	Actor     string         `json:"actor"`
	Kind      string         `json:"kind"`
	Payload   map[string]any `json:"payload"`
}

// WriteBundle writes run.json, an append-only lifecycle JSONL stream, a
// manifest, and checksums for every material file in the bundle.
func WriteBundle(outputDir string, record runner.RunRecord, sealedPolicy *policy.Sealed) (Bundle, error) {
	return WriteBundleWithPolicies(outputDir, record, sealedPolicy, nil)
}

// WriteBundleWithPolicies keeps the oracle-generation policy and the managed
// subject policy as separate evidence inputs. The old WriteBundle API remains
// a compatibility path for runs that only have one policy.
func WriteBundleWithPolicies(outputDir string, record runner.RunRecord, sealedPolicy, sealedSubjectPolicy *policy.Sealed) (Bundle, error) {
	if strings.TrimSpace(outputDir) == "" {
		return Bundle{}, fmt.Errorf("evidence output directory must not be empty")
	}
	if record.Lifecycle != nil && record.Lifecycle.Access != nil {
		access := record.Lifecycle.Access
		if err := validateExecutableSampling(
			access.ExecutableSampleCount,
			access.ExecutableSamplingIntervalMS,
			access.ExecutableSamplingStartedAt,
			access.ExecutableSamplingStoppedAt,
			access.ExecutableObservationCount,
			access.ExecutableObservationErrors,
		); err != nil {
			return Bundle{}, fmt.Errorf("invalid executable sampling metadata: %w", err)
		}
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return Bundle{}, err
	}
	runPath, err := runner.Write(outputDir, record)
	if err != nil {
		return Bundle{}, err
	}
	eventsDir := filepath.Join(outputDir, "events")
	if err := os.MkdirAll(eventsDir, 0o755); err != nil {
		return Bundle{}, err
	}
	lifecyclePath := filepath.Join(eventsDir, "lifecycle.jsonl")
	if err := writeLifecycleEvents(lifecyclePath, record); err != nil {
		return Bundle{}, err
	}

	runHash, err := hashFile(runPath)
	if err != nil {
		return Bundle{}, fmt.Errorf("hash run.json: %w", err)
	}
	lifecycleHash, err := hashFile(lifecyclePath)
	if err != nil {
		return Bundle{}, fmt.Errorf("hash events/lifecycle.jsonl: %w", err)
	}
	artifacts := map[string]string{
		"run.json":               runHash,
		"events/lifecycle.jsonl": lifecycleHash,
	}
	var subjectAccessPath string
	var subjectExecutablePath string
	if record.Lifecycle != nil && record.Lifecycle.Access != nil {
		subjectAccessPath = filepath.Join(eventsDir, "subject-access.jsonl")
		if err := writeSubjectAccessEvents(subjectAccessPath, record); err != nil {
			return Bundle{}, err
		}
		subjectAccessHash, err := hashFile(subjectAccessPath)
		if err != nil {
			return Bundle{}, fmt.Errorf("hash events/subject-access.jsonl: %w", err)
		}
		artifacts["events/subject-access.jsonl"] = subjectAccessHash
		subjectExecutablePath = filepath.Join(eventsDir, "subject-executables.jsonl")
		if err := writeSubjectExecutableObservations(subjectExecutablePath, record); err != nil {
			return Bundle{}, err
		}
		subjectExecutableHash, err := hashFile(subjectExecutablePath)
		if err != nil {
			return Bundle{}, fmt.Errorf("hash events/subject-executables.jsonl: %w", err)
		}
		artifacts["events/subject-executables.jsonl"] = subjectExecutableHash
	}
	var policyReference *policy.Reference
	if sealedPolicy != nil {
		policyDir := filepath.Join(outputDir, "policy")
		if err := os.MkdirAll(policyDir, 0o755); err != nil {
			return Bundle{}, err
		}
		policyCanonicalPath := filepath.Join(policyDir, "canonical.json")
		if err := os.WriteFile(policyCanonicalPath, sealedPolicy.CanonicalJSON, 0o644); err != nil {
			return Bundle{}, err
		}
		policyHashPath := filepath.Join(policyDir, "hash.txt")
		if err := os.WriteFile(policyHashPath, []byte(sealedPolicy.SHA256+"\n"), 0o644); err != nil {
			return Bundle{}, err
		}
		policyCanonicalHash, err := hashFile(policyCanonicalPath)
		if err != nil {
			return Bundle{}, fmt.Errorf("hash policy/canonical.json: %w", err)
		}
		policyHashHash, err := hashFile(policyHashPath)
		if err != nil {
			return Bundle{}, fmt.Errorf("hash policy/hash.txt: %w", err)
		}
		artifacts["policy/canonical.json"] = policyCanonicalHash
		artifacts["policy/hash.txt"] = policyHashHash
		reference := sealedPolicy.Reference()
		policyReference = &reference
	}
	var subjectPolicyReference *policy.Reference
	if sealedSubjectPolicy != nil {
		policyDir := filepath.Join(outputDir, "policy", "subject")
		if err := os.MkdirAll(policyDir, 0o755); err != nil {
			return Bundle{}, err
		}
		policyCanonicalPath := filepath.Join(policyDir, "canonical.json")
		if err := os.WriteFile(policyCanonicalPath, sealedSubjectPolicy.CanonicalJSON, 0o644); err != nil {
			return Bundle{}, err
		}
		policyHashPath := filepath.Join(policyDir, "hash.txt")
		if err := os.WriteFile(policyHashPath, []byte(sealedSubjectPolicy.SHA256+"\n"), 0o644); err != nil {
			return Bundle{}, err
		}
		policyCanonicalHash, err := hashFile(policyCanonicalPath)
		if err != nil {
			return Bundle{}, fmt.Errorf("hash policy/subject/canonical.json: %w", err)
		}
		policyHashHash, err := hashFile(policyHashPath)
		if err != nil {
			return Bundle{}, fmt.Errorf("hash policy/subject/hash.txt: %w", err)
		}
		artifacts["policy/subject/canonical.json"] = policyCanonicalHash
		artifacts["policy/subject/hash.txt"] = policyHashHash
		reference := sealedSubjectPolicy.Reference()
		subjectPolicyReference = &reference
	}
	manifest := Manifest{
		Schema:          "sorna.evidence/v1",
		RunID:           record.RunID,
		CreatedAt:       record.CreatedAt,
		Assurance:       record.Assurance,
		Contract:        record.Contract,
		Oracle:          record.Oracle,
		Baseline:        record.Baseline,
		Subject:         record.Subject,
		Policy:          policyReference,
		SubjectPolicy:   subjectPolicyReference,
		ArtifactsSHA256: artifacts,
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Bundle{}, fmt.Errorf("encode evidence manifest: %w", err)
	}
	manifestBytes = append(manifestBytes, '\n')
	manifestPath := filepath.Join(outputDir, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		return Bundle{}, err
	}
	artifacts["manifest.json"] = hashBytes(manifestBytes)

	checksumsPath := filepath.Join(outputDir, "checksums.sha256")
	if err := writeChecksums(checksumsPath, artifacts); err != nil {
		return Bundle{}, err
	}
	return Bundle{
		RootDir:               outputDir,
		RunPath:               runPath,
		ManifestPath:          manifestPath,
		LifecyclePath:         lifecyclePath,
		SubjectAccessPath:     subjectAccessPath,
		SubjectExecutablePath: subjectExecutablePath,
		ChecksumsPath:         checksumsPath,
	}, nil
}

// AttachCampaignProvenance adds the exact campaign plan and provider bytes to
// an already-written mutation evidence bundle. It is intentionally a separate
// step because the child run writer knows about the run, while the campaign
// executor knows which plan and provider selected that run.
func AttachCampaignProvenance(outputDir string, input CampaignProvenanceInput) error {
	if strings.TrimSpace(outputDir) == "" {
		return fmt.Errorf("evidence output directory must not be empty")
	}
	if input.Sequence < 1 {
		return fmt.Errorf("campaign sequence must be positive")
	}
	if strings.TrimSpace(input.MutationID) == "" {
		return fmt.Errorf("campaign mutation ID must be non-empty")
	}
	if err := Verify(outputDir); err != nil {
		return fmt.Errorf("verify evidence before attaching campaign provenance: %w", err)
	}

	manifestPath := filepath.Join(outputDir, "manifest.json")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read evidence manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("decode evidence manifest: %w", err)
	}
	if manifest.Schema != "sorna.evidence/v1" {
		return fmt.Errorf("campaign provenance requires a Sorna run evidence bundle, got %q", manifest.Schema)
	}
	if manifest.Campaign != nil {
		return fmt.Errorf("evidence bundle already contains campaign provenance")
	}

	planBytes := input.PlanBytes
	if planBytes == nil {
		planBytes, err = os.ReadFile(input.PlanPath)
		if err != nil {
			return fmt.Errorf("read campaign plan: %w", err)
		}
	}
	providerBytes := input.ProviderBytes
	if providerBytes == nil {
		providerBytes, err = os.ReadFile(input.ProviderPath)
		if err != nil {
			return fmt.Errorf("read mutation provider: %w", err)
		}
	}
	planReference := CampaignArtifactReference{
		SourcePath: input.PlanPath,
		BundlePath: "campaign/plan.json",
		SHA256:     hashBytes(planBytes),
	}
	providerReference := CampaignArtifactReference{
		SourcePath: input.ProviderPath,
		BundlePath: "campaign/provider.yaml",
		SHA256:     hashBytes(providerBytes),
	}
	provenance := CampaignProvenance{
		Schema:     campaignProvenanceSchema,
		RunID:      manifest.RunID,
		Sequence:   input.Sequence,
		MutationID: input.MutationID,
		Plan:       planReference,
		Provider:   providerReference,
	}
	if err := validateCampaignProvenance(provenance, manifest.RunID); err != nil {
		return fmt.Errorf("invalid campaign provenance: %w", err)
	}

	campaignDir := filepath.Join(outputDir, "campaign")
	if _, err := os.Stat(campaignDir); err == nil {
		return fmt.Errorf("campaign evidence directory already exists")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect campaign evidence directory: %w", err)
	}
	if err := os.Mkdir(campaignDir, 0o755); err != nil {
		return fmt.Errorf("create campaign evidence directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, planReference.BundlePath), planBytes, 0o644); err != nil {
		return fmt.Errorf("write campaign plan evidence: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, providerReference.BundlePath), providerBytes, 0o644); err != nil {
		return fmt.Errorf("write campaign provider evidence: %w", err)
	}
	provenanceBytes, err := marshalJSON(provenance)
	if err != nil {
		return fmt.Errorf("encode campaign provenance: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "campaign/provenance.json"), provenanceBytes, 0o644); err != nil {
		return fmt.Errorf("write campaign provenance evidence: %w", err)
	}

	artifacts := cloneArtifacts(manifest.ArtifactsSHA256)
	artifacts[planReference.BundlePath] = hashBytes(planBytes)
	artifacts[providerReference.BundlePath] = hashBytes(providerBytes)
	artifacts["campaign/provenance.json"] = hashBytes(provenanceBytes)
	manifest.Campaign = &provenance
	manifest.ArtifactsSHA256 = artifacts
	updatedManifestBytes, err := marshalJSON(manifest)
	if err != nil {
		return fmt.Errorf("encode updated evidence manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, updatedManifestBytes, 0o644); err != nil {
		return fmt.Errorf("write updated evidence manifest: %w", err)
	}
	artifacts["manifest.json"] = hashBytes(updatedManifestBytes)
	if err := writeChecksums(filepath.Join(outputDir, "checksums.sha256"), artifacts); err != nil {
		return fmt.Errorf("rewrite evidence checksums: %w", err)
	}
	if err := Verify(outputDir); err != nil {
		return fmt.Errorf("verify evidence after attaching campaign provenance: %w", err)
	}
	return nil
}

// Verify checks every path listed in checksums.sha256 and reports the first
// missing, malformed, duplicated, or mismatched artifact.
func Verify(outputDir string) error {
	if strings.TrimSpace(outputDir) == "" {
		return fmt.Errorf("evidence output directory must not be empty")
	}
	contents, err := os.ReadFile(filepath.Join(outputDir, "checksums.sha256"))
	if err != nil {
		return err
	}
	seen := make(map[string]bool)
	lines := strings.Split(strings.TrimSpace(string(contents)), "\n")
	for lineNumber, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 || len(parts[0]) != sha256.Size*2 || parts[1] == "" {
			return fmt.Errorf("invalid checksum line %d", lineNumber+1)
		}
		if _, err := hex.DecodeString(parts[0]); err != nil {
			return fmt.Errorf("invalid checksum line %d: %w", lineNumber+1, err)
		}
		relative, err := safeRelativePath(parts[1])
		if err != nil {
			return fmt.Errorf("checksum line %d: %w", lineNumber+1, err)
		}
		if seen[relative] {
			return fmt.Errorf("duplicate checksum path %q", relative)
		}
		seen[relative] = true
		actual, err := hashFile(filepath.Join(outputDir, relative))
		if err != nil {
			return fmt.Errorf("hash %s: %w", relative, err)
		}
		if actual != parts[0] {
			return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", relative, parts[0], actual)
		}
	}
	if len(seen) == 0 {
		return fmt.Errorf("checksum file contains no artifacts")
	}
	return verifyBundleSemantics(outputDir, seen)
}

func validateExecutableSampling(sampleCount, intervalMS int, startedAt, stoppedAt time.Time, observationCount, observationErrors int) error {
	if sampleCount < 0 {
		return fmt.Errorf("sample count must not be negative")
	}
	if intervalMS < 0 {
		return fmt.Errorf("sampling interval must not be negative")
	}
	if observationCount < 0 {
		return fmt.Errorf("observation count must not be negative")
	}
	if observationErrors < 0 {
		return fmt.Errorf("observation error count must not be negative")
	}
	if sampleCount == 0 {
		if intervalMS != 0 || !startedAt.IsZero() || !stoppedAt.IsZero() {
			return fmt.Errorf("sampling window is present without a sample attempt")
		}
		if observationCount != 0 || observationErrors != 0 {
			return fmt.Errorf("observations or errors are present without a sample attempt")
		}
		return nil
	}
	if intervalMS <= 0 {
		return fmt.Errorf("sampling interval must be positive when samples were attempted")
	}
	if startedAt.IsZero() || stoppedAt.IsZero() {
		return fmt.Errorf("sampling window timestamps are required when samples were attempted")
	}
	if stoppedAt.Before(startedAt) {
		return fmt.Errorf("sampling window stopped before it started")
	}
	return nil
}

func verifyBundleSemantics(outputDir string, checksums map[string]bool) error {
	manifestBytes, err := os.ReadFile(filepath.Join(outputDir, "manifest.json"))
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	var envelope struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(manifestBytes, &envelope); err != nil {
		return fmt.Errorf("decode manifest: %w", err)
	}
	switch envelope.Schema {
	case "sorna.oracle-evidence/v1":
		var manifest OracleManifest
		if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
			return fmt.Errorf("decode oracle manifest: %w", err)
		}
		if err := validateExecutableSampling(
			manifest.Execution.Access.ExecutableSampleCount,
			manifest.Execution.Access.ExecutableSamplingIntervalMS,
			manifest.Execution.Access.ExecutableSamplingStartedAt,
			manifest.Execution.Access.ExecutableSamplingStoppedAt,
			manifest.Execution.Access.ExecutableObservationCount,
			manifest.Execution.Access.ExecutableObservationErrors,
		); err != nil {
			return fmt.Errorf("invalid oracle executable sampling metadata: %w", err)
		}
		return verifyExecutableObservationStream(
			filepath.Join(outputDir, "events", "executables.jsonl"),
			checksums["events/executables.jsonl"],
			manifest.Execution.Access.ExecutableObservationCount,
			manifest.Execution.Access.ExecutableSamplingStartedAt,
			manifest.Execution.Access.ExecutableSamplingStoppedAt,
			func(line []byte) error {
				var observation OracleExecutableObservation
				if err := json.Unmarshal(line, &observation); err != nil {
					return err
				}
				if observation.EventID == "" || observation.ExecutionID == "" || observation.PID <= 0 || observation.Path == "" || observation.SHA256 == "" || observation.Timestamp.IsZero() {
					return fmt.Errorf("observation identity fields are incomplete")
				}
				return nil
			},
		)
	case "sorna.evidence/v1":
		var manifest Manifest
		if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
			return fmt.Errorf("decode evidence manifest: %w", err)
		}
		runBytes, err := os.ReadFile(filepath.Join(outputDir, "run.json"))
		if err != nil {
			return fmt.Errorf("read run: %w", err)
		}
		var record runner.RunRecord
		if err := json.Unmarshal(runBytes, &record); err != nil {
			return fmt.Errorf("decode run: %w", err)
		}
		if record.Schema != "ingen.run/v1" || manifest.RunID != record.RunID || manifest.Contract != record.Contract || !sameOracle(manifest.Oracle, record.Oracle) || manifest.Subject != record.Subject || !sameBaseline(manifest.Baseline, record.Baseline) {
			return fmt.Errorf("evidence manifest identity does not match run identity")
		}
		if err := verifyCampaignProvenance(outputDir, manifest, checksums); err != nil {
			return err
		}
		if record.Lifecycle == nil || record.Lifecycle.Access == nil {
			return nil
		}
		access := record.Lifecycle.Access
		if err := validateExecutableSampling(
			access.ExecutableSampleCount,
			access.ExecutableSamplingIntervalMS,
			access.ExecutableSamplingStartedAt,
			access.ExecutableSamplingStoppedAt,
			access.ExecutableObservationCount,
			access.ExecutableObservationErrors,
		); err != nil {
			return fmt.Errorf("invalid subject executable sampling metadata: %w", err)
		}
		return verifyExecutableObservationStream(
			filepath.Join(outputDir, "events", "subject-executables.jsonl"),
			checksums["events/subject-executables.jsonl"],
			access.ExecutableObservationCount,
			access.ExecutableSamplingStartedAt,
			access.ExecutableSamplingStoppedAt,
			func(line []byte) error {
				var observation SubjectExecutableObservation
				if err := json.Unmarshal(line, &observation); err != nil {
					return err
				}
				if observation.EventID == "" || observation.RunID == "" || observation.PID <= 0 || observation.Path == "" || observation.SHA256 == "" || observation.Timestamp.IsZero() {
					return fmt.Errorf("observation identity fields are incomplete")
				}
				return nil
			},
		)
	default:
		return nil
	}
}

func verifyCampaignProvenance(outputDir string, manifest Manifest, checksums map[string]bool) error {
	campaignPaths := []string{"campaign/plan.json", "campaign/provider.yaml", "campaign/provenance.json"}
	hasCampaignArtifacts := false
	for _, path := range campaignPaths {
		if checksums[path] {
			hasCampaignArtifacts = true
			break
		}
	}
	if manifest.Campaign == nil {
		if hasCampaignArtifacts {
			return fmt.Errorf("campaign artifacts are present without campaign provenance")
		}
		return nil
	}
	if err := validateCampaignProvenance(*manifest.Campaign, manifest.RunID); err != nil {
		return fmt.Errorf("invalid campaign provenance: %w", err)
	}
	if !checksums["campaign/plan.json"] || !checksums["campaign/provider.yaml"] || !checksums["campaign/provenance.json"] {
		return fmt.Errorf("campaign provenance artifacts are not all checksummed")
	}
	provenanceBytes, err := os.ReadFile(filepath.Join(outputDir, "campaign/provenance.json"))
	if err != nil {
		return fmt.Errorf("read campaign provenance: %w", err)
	}
	var provenance CampaignProvenance
	if err := json.Unmarshal(provenanceBytes, &provenance); err != nil {
		return fmt.Errorf("decode campaign provenance: %w", err)
	}
	if provenance != *manifest.Campaign {
		return fmt.Errorf("campaign provenance does not match evidence manifest")
	}
	for _, reference := range []CampaignArtifactReference{manifest.Campaign.Plan, manifest.Campaign.Provider} {
		actual, err := hashFile(filepath.Join(outputDir, reference.BundlePath))
		if err != nil {
			return fmt.Errorf("hash campaign artifact %s: %w", reference.BundlePath, err)
		}
		if actual != reference.SHA256 {
			return fmt.Errorf("campaign artifact %s hash does not match provenance", reference.BundlePath)
		}
		if manifest.ArtifactsSHA256[reference.BundlePath] != reference.SHA256 {
			return fmt.Errorf("campaign artifact %s hash does not match manifest", reference.BundlePath)
		}
	}
	if manifest.ArtifactsSHA256["campaign/provenance.json"] != hashBytes(provenanceBytes) {
		return fmt.Errorf("campaign provenance hash does not match manifest")
	}
	return nil
}

func validateCampaignProvenance(provenance CampaignProvenance, runID string) error {
	if provenance.Schema != campaignProvenanceSchema {
		return fmt.Errorf("schema must be %s", campaignProvenanceSchema)
	}
	if strings.TrimSpace(provenance.RunID) == "" || provenance.RunID != runID {
		return fmt.Errorf("run ID must match evidence run")
	}
	if provenance.Sequence < 1 {
		return fmt.Errorf("sequence must be positive")
	}
	if strings.TrimSpace(provenance.MutationID) == "" {
		return fmt.Errorf("mutation ID must be non-empty")
	}
	for name, reference := range map[string]CampaignArtifactReference{"plan": provenance.Plan, "provider": provenance.Provider} {
		if strings.TrimSpace(reference.SourcePath) == "" {
			return fmt.Errorf("%s source path must be non-empty", name)
		}
		if reference.BundlePath == "" {
			return fmt.Errorf("%s bundle path must be non-empty", name)
		}
		if _, err := safeRelativePath(reference.BundlePath); err != nil {
			return fmt.Errorf("%s bundle path: %w", name, err)
		}
		if reference.SHA256 == "" || len(reference.SHA256) != sha256.Size*2 {
			return fmt.Errorf("%s hash must be a SHA-256 digest", name)
		}
		if _, err := hex.DecodeString(reference.SHA256); err != nil {
			return fmt.Errorf("%s hash must be hexadecimal: %w", name, err)
		}
	}
	if provenance.Plan.BundlePath != "campaign/plan.json" {
		return fmt.Errorf("plan bundle path must be campaign/plan.json")
	}
	if provenance.Provider.BundlePath != "campaign/provider.yaml" {
		return fmt.Errorf("provider bundle path must be campaign/provider.yaml")
	}
	return nil
}

func cloneArtifacts(artifacts map[string]string) map[string]string {
	clone := make(map[string]string, len(artifacts)+4)
	for path, digest := range artifacts {
		clone[path] = digest
	}
	return clone
}

func marshalJSON(value any) ([]byte, error) {
	contents, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(contents, '\n'), nil
}

func verifyExecutableObservationStream(path string, listed bool, expected int, startedAt, stoppedAt time.Time, validate func([]byte) error) error {
	if !listed {
		return fmt.Errorf("executable observation stream is not checksummed")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read executable observation stream: %w", err)
	}
	trimmed := strings.TrimSpace(string(contents))
	if trimmed == "" {
		if expected != 0 {
			return fmt.Errorf("executable observation stream contains 0 records, expected %d", expected)
		}
		return nil
	}
	count := 0
	for lineNumber, line := range strings.Split(trimmed, "\n") {
		if err := validate([]byte(line)); err != nil {
			return fmt.Errorf("invalid executable observation line %d: %w", lineNumber+1, err)
		}
		var envelope struct {
			Timestamp time.Time `json:"timestamp"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			return fmt.Errorf("invalid executable observation line %d: %w", lineNumber+1, err)
		}
		if !startedAt.IsZero() && (envelope.Timestamp.Before(startedAt) || envelope.Timestamp.After(stoppedAt)) {
			return fmt.Errorf("executable observation line %d timestamp is outside the sampling window", lineNumber+1)
		}
		count++
	}
	if count != expected {
		return fmt.Errorf("executable observation stream contains %d records, expected %d", count, expected)
	}
	return nil
}

func writeLifecycleEvents(path string, record runner.RunRecord) error {
	var buffer bytes.Buffer
	if record.Lifecycle != nil {
		for _, event := range record.Lifecycle.Events {
			line := LifecycleEvent{
				EventID:   fmt.Sprintf("evt-%04d", event.Sequence),
				RunID:     record.RunID,
				Sequence:  event.Sequence,
				Timestamp: event.Timestamp,
				Actor:     "sorna",
				Kind:      event.Kind,
				Payload:   map[string]any{},
			}
			if event.Detail != "" {
				line.Payload["detail"] = event.Detail
			}
			encoded, err := json.Marshal(line)
			if err != nil {
				return fmt.Errorf("encode lifecycle event %d: %w", event.Sequence, err)
			}
			buffer.Write(encoded)
			buffer.WriteByte('\n')
		}
	}
	return os.WriteFile(path, buffer.Bytes(), 0o644)
}

type SubjectAccessEvent struct {
	EventID   string    `json:"event_id"`
	RunID     string    `json:"run_id"`
	Sequence  int       `json:"sequence"`
	Timestamp time.Time `json:"timestamp"`
	Process   string    `json:"process"`
	PID       int       `json:"pid"`
	Decision  string    `json:"decision"`
	Operation string    `json:"operation"`
	Resource  string    `json:"resource"`
}

func writeSubjectAccessEvents(path string, record runner.RunRecord) error {
	var buffer bytes.Buffer
	if record.Lifecycle != nil {
		for index, event := range record.Lifecycle.AccessEvents {
			line := SubjectAccessEvent{
				EventID:   fmt.Sprintf("access-%04d", index+1),
				RunID:     record.RunID,
				Sequence:  index + 1,
				Timestamp: event.Timestamp,
				Process:   event.Process,
				PID:       event.PID,
				Decision:  event.Decision,
				Operation: event.Operation,
				Resource:  event.Resource,
			}
			encoded, err := json.Marshal(line)
			if err != nil {
				return fmt.Errorf("encode subject access event %d: %w", index+1, err)
			}
			buffer.Write(encoded)
			buffer.WriteByte('\n')
		}
	}
	return os.WriteFile(path, buffer.Bytes(), 0o644)
}

type SubjectExecutableObservation struct {
	EventID   string    `json:"event_id"`
	RunID     string    `json:"run_id"`
	Sequence  int       `json:"sequence"`
	Timestamp time.Time `json:"timestamp"`
	PID       int       `json:"pid"`
	Path      string    `json:"path"`
	SHA256    string    `json:"sha256"`
}

func writeSubjectExecutableObservations(path string, record runner.RunRecord) error {
	var buffer bytes.Buffer
	if record.Lifecycle != nil {
		for index, observation := range record.Lifecycle.ExecutableObservations {
			line := SubjectExecutableObservation{
				EventID:   fmt.Sprintf("executable-%04d", index+1),
				RunID:     record.RunID,
				Sequence:  index + 1,
				Timestamp: observation.Timestamp,
				PID:       observation.PID,
				Path:      observation.Path,
				SHA256:    observation.SHA256,
			}
			encoded, err := json.Marshal(line)
			if err != nil {
				return fmt.Errorf("encode subject executable observation %d: %w", index+1, err)
			}
			buffer.Write(encoded)
			buffer.WriteByte('\n')
		}
	}
	return os.WriteFile(path, buffer.Bytes(), 0o644)
}

func writeChecksums(path string, artifacts map[string]string) error {
	paths := make([]string, 0, len(artifacts))
	for relative := range artifacts {
		paths = append(paths, relative)
	}
	sort.Strings(paths)
	var buffer strings.Builder
	for _, relative := range paths {
		if _, err := safeRelativePath(relative); err != nil {
			return err
		}
		fmt.Fprintf(&buffer, "%s  %s\n", artifacts[relative], relative)
	}
	return os.WriteFile(path, []byte(buffer.String()), 0o644)
}

func hashFile(path string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return hashBytes(contents), nil
}

func hashBytes(contents []byte) string {
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:])
}

func safeRelativePath(raw string) (string, error) {
	clean := filepath.Clean(raw)
	if clean == "." || filepath.IsAbs(raw) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("checksum path must stay inside the evidence bundle: %q", raw)
	}
	return clean, nil
}
