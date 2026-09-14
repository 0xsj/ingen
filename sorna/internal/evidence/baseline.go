package evidence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"ingen/sorna/internal/policy"
	"ingen/sorna/internal/runner"
)

// BaselineRequirements are the immutable inputs a mutation run must share
// with its clean baseline.
type BaselineRequirements struct {
	Contract            runner.ContractReference
	Oracle              *runner.OracleReference
	PolicySHA256        string
	SubjectPolicySHA256 string
}

// ValidateBaseline verifies that outputDir is a passing, unmutated run using
// the same contract, oracle, and policy identities as the candidate run.
func ValidateBaseline(outputDir string, requirements BaselineRequirements) (runner.BaselineReference, error) {
	if err := Verify(outputDir); err != nil {
		return runner.BaselineReference{}, fmt.Errorf("verify baseline evidence: %w", err)
	}
	manifest, record, err := loadRun(outputDir)
	if err != nil {
		return runner.BaselineReference{}, err
	}
	if record.Verdict.Status != "pass" {
		return runner.BaselineReference{}, fmt.Errorf("baseline contract verdict is %q, want pass", record.Verdict.Status)
	}
	if record.Mutation != nil {
		return runner.BaselineReference{}, fmt.Errorf("baseline run contains mutation %q", record.Mutation.Spec.ID)
	}
	if record.Contract != requirements.Contract {
		return runner.BaselineReference{}, fmt.Errorf("baseline contract %s@%d (%s) does not match candidate %s@%d (%s)",
			record.Contract.ID, record.Contract.Version, record.Contract.SHA256,
			requirements.Contract.ID, requirements.Contract.Version, requirements.Contract.SHA256)
	}
	if !sameOracle(record.Oracle, requirements.Oracle) {
		return runner.BaselineReference{}, fmt.Errorf("baseline oracle does not match candidate oracle")
	}
	if policyHash(manifest.Policy) != requirements.PolicySHA256 {
		return runner.BaselineReference{}, fmt.Errorf("baseline oracle policy hash %q does not match candidate policy hash %q", policyHash(manifest.Policy), requirements.PolicySHA256)
	}
	if policyHash(manifest.SubjectPolicy) != requirements.SubjectPolicySHA256 {
		return runner.BaselineReference{}, fmt.Errorf("baseline subject policy hash %q does not match candidate subject policy hash %q", policyHash(manifest.SubjectPolicy), requirements.SubjectPolicySHA256)
	}
	return runner.BaselineReference{
		EvidencePath: outputDir,
		RunID:        record.RunID,
		Contract:     record.Contract,
		Oracle:       cloneOracle(record.Oracle),
	}, nil
}

func loadRun(outputDir string) (Manifest, runner.RunRecord, error) {
	manifestBytes, err := os.ReadFile(filepath.Join(outputDir, "manifest.json"))
	if err != nil {
		return Manifest{}, runner.RunRecord{}, fmt.Errorf("read baseline manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return Manifest{}, runner.RunRecord{}, fmt.Errorf("decode baseline manifest: %w", err)
	}
	if manifest.Schema != "sorna.evidence/v1" {
		return Manifest{}, runner.RunRecord{}, fmt.Errorf("baseline manifest schema must be sorna.evidence/v1, got %q", manifest.Schema)
	}
	runBytes, err := os.ReadFile(filepath.Join(outputDir, "run.json"))
	if err != nil {
		return Manifest{}, runner.RunRecord{}, fmt.Errorf("read baseline run: %w", err)
	}
	var record runner.RunRecord
	if err := json.Unmarshal(runBytes, &record); err != nil {
		return Manifest{}, runner.RunRecord{}, fmt.Errorf("decode baseline run: %w", err)
	}
	if record.Schema != "ingen.run/v1" {
		return Manifest{}, runner.RunRecord{}, fmt.Errorf("baseline run schema must be ingen.run/v1, got %q", record.Schema)
	}
	if manifest.RunID != record.RunID || manifest.Contract != record.Contract || !sameOracle(manifest.Oracle, record.Oracle) {
		return Manifest{}, runner.RunRecord{}, fmt.Errorf("baseline manifest identity does not match run identity")
	}
	return manifest, record, nil
}

func policyHash(reference *policy.Reference) string {
	if reference == nil {
		return ""
	}
	return reference.SHA256
}

func sameOracle(left, right *runner.OracleReference) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func cloneOracle(reference *runner.OracleReference) *runner.OracleReference {
	if reference == nil {
		return nil
	}
	copy := *reference
	return &copy
}

func sameBaseline(left, right *runner.BaselineReference) bool {
	return reflect.DeepEqual(left, right)
}
