package evidence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ingen/core/ciresult"
	"ingen/sorna/internal/campaign"
	"ingen/sorna/internal/contract"
	"ingen/sorna/internal/mutation"
	"ingen/sorna/internal/oracle"
)

const contractMutationExplanationSchema = "sorna.contract-mutation-explanation/v1"

// ContractMutationExplanation is the compact CI-facing interpretation of a
// contract-mutation report. The complete producer-owned report remains in the
// envelope's Report field.
type ContractMutationExplanation struct {
	Schema           string                           `json:"schema"`
	Status           string                           `json:"status"`
	Summary          mutation.ContractMutationSummary `json:"summary"`
	VisibleMutations []string                         `json:"visible_mutations,omitempty"`
	InvalidMutations []string                         `json:"invalid_mutations,omitempty"`
}

// BuildContractMutationCIResult verifies a contract-mutation report against
// the original contract and frozen oracle before adapting it to the shared CI
// envelope. A failed result means the report contains invalid mutations; an
// error result is reserved for unreadable, non-canonical, or mismatched
// artifacts.
func BuildContractMutationCIResult(reportPath, contractPath, oraclePath, sourceRoot string) (ciresult.Artifact, error) {
	if strings.TrimSpace(reportPath) == "" || strings.TrimSpace(contractPath) == "" || strings.TrimSpace(oraclePath) == "" {
		return ciresult.Artifact{}, fmt.Errorf("contract mutation report, contract, and oracle paths must not be empty")
	}
	if strings.TrimSpace(sourceRoot) == "" {
		sourceRoot = "."
	}
	reportResolved := resolveContractMutationPath(reportPath, sourceRoot)
	contractResolved := resolveContractMutationPath(contractPath, sourceRoot)
	oracleResolved := resolveContractMutationPath(oraclePath, sourceRoot)
	reportBytes, err := os.ReadFile(reportResolved)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("read contract mutation report: %w", err)
	}
	report, err := mutation.LoadContractMutationReport(reportResolved)
	if err != nil {
		return ciresult.Artifact{}, err
	}
	contractDocument, err := contract.LoadFile(contractResolved)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("load contract mutation contract: %w", err)
	}
	contractAbsolute, err := filepath.Abs(contractResolved)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("resolve contract mutation contract: %w", err)
	}
	sealed, err := contract.SealAt(contractDocument, filepath.Dir(contractAbsolute))
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("seal contract mutation contract: %w", err)
	}
	originalOracle, err := oracle.LoadFile(oracleResolved)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("load contract mutation oracle: %w", err)
	}
	oracleHash, err := oracle.HashFile(oracleResolved)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("hash contract mutation oracle: %w", err)
	}
	if report.Contract.ID != originalOracle.Contract.ID || report.Contract.Version != originalOracle.Contract.Version || report.Contract.SHA256 != sealed.SHA256 {
		return ciresult.Artifact{}, fmt.Errorf("contract mutation report does not match the supplied contract and oracle")
	}
	if report.Oracle.Schema != originalOracle.Schema || report.Oracle.SHA256 != oracleHash || report.Oracle.PolicySHA256 != originalOracle.PolicySHA256 {
		return ciresult.Artifact{}, fmt.Errorf("contract mutation report oracle reference does not match the supplied oracle")
	}
	status := "passed"
	if report.Summary.Invalid > 0 {
		status = "failed"
	}
	exitCode, err := ciresult.ExitCodeForStatus(status)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("map contract mutation status: %w", err)
	}
	explanation := buildContractMutationExplanation(report)
	explanationBytes, err := json.Marshal(explanation)
	if err != nil {
		return ciresult.Artifact{}, fmt.Errorf("encode contract mutation explanation: %w", err)
	}
	artifact := ciresult.Artifact{
		Schema:    ciresult.Schema,
		Tool:      "sorna",
		Kind:      "contract-mutation-inspection",
		Status:    status,
		ExitCode:  exitCode,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Source: ciresult.Source{
			Root:       sourceRoot,
			ModulePath: report.Contract.ID,
		},
		Inputs: map[string]ciresult.FileRef{
			"report":   {Path: reportPath, SHA256: campaign.HashBytes(reportBytes)},
			"contract": {Path: contractPath, SHA256: sealed.SHA256},
			"oracle":   {Path: oraclePath, SHA256: oracleHash},
		},
		Report:      reportBytes,
		Explanation: explanationBytes,
	}
	if err := artifact.Validate(); err != nil {
		return ciresult.Artifact{}, fmt.Errorf("validate contract mutation CI result: %w", err)
	}
	return artifact, nil
}

// BuildContractMutationCIErrorResult produces a collector-friendly error
// envelope when the report or its original artifact bindings cannot be
// verified.
func BuildContractMutationCIErrorResult(reportPath, contractPath, oraclePath, sourceRoot string, cause error) (ciresult.Artifact, error) {
	if cause == nil {
		return ciresult.Artifact{}, fmt.Errorf("contract mutation CI error result requires an error")
	}
	if strings.TrimSpace(sourceRoot) == "" {
		sourceRoot = "."
	}
	artifact := ciresult.Artifact{
		Schema:    ciresult.Schema,
		Tool:      "sorna",
		Kind:      "contract-mutation-inspection",
		Status:    "error",
		ExitCode:  2,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Source:    ciresult.Source{Root: sourceRoot},
		Error:     cause.Error(),
		Inputs:    make(map[string]ciresult.FileRef),
	}
	for name, path := range map[string]string{
		"report":   reportPath,
		"contract": contractPath,
		"oracle":   oraclePath,
	} {
		if strings.TrimSpace(path) == "" {
			continue
		}
		resolved := resolveContractMutationPath(path, sourceRoot)
		if hash, err := campaign.HashFile(resolved); err == nil {
			artifact.Inputs[name] = ciresult.FileRef{Path: path, SHA256: hash}
		}
	}
	if err := artifact.Validate(); err != nil {
		return ciresult.Artifact{}, fmt.Errorf("validate contract mutation CI error result: %w", err)
	}
	return artifact, nil
}

func buildContractMutationExplanation(report mutation.ContractMutationReport) ContractMutationExplanation {
	explanation := ContractMutationExplanation{
		Schema:  contractMutationExplanationSchema,
		Status:  report.Status,
		Summary: report.Summary,
	}
	for _, entry := range report.Entries {
		switch entry.Outcome {
		case "visible":
			explanation.VisibleMutations = append(explanation.VisibleMutations, entry.ID)
		case "invalid":
			explanation.InvalidMutations = append(explanation.InvalidMutations, entry.ID)
		}
	}
	return explanation
}

func resolveContractMutationPath(path, sourceRoot string) string {
	if filepath.IsAbs(path) || sourceRoot == "." {
		return path
	}
	return filepath.Join(sourceRoot, path)
}
