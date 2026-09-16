package runner

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"ingen/sorna/internal/oracle"
)

// LoadFile loads a run record with the v1 decoder policy. Unknown fields and
// trailing JSON values are rejected so a consumer cannot silently read a
// different artifact shape than the published run contract.
func LoadFile(path string) (RunRecord, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return RunRecord{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var record RunRecord
	if err := decoder.Decode(&record); err != nil {
		return RunRecord{}, fmt.Errorf("decode run record: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return RunRecord{}, fmt.Errorf("decode run record: multiple JSON values")
		}
		return RunRecord{}, fmt.Errorf("decode run record trailing data: %w", err)
	}
	if err := record.Validate(); err != nil {
		return RunRecord{}, err
	}
	return record, nil
}

// Validate checks the identity and lineage fields that a consumer needs
// before interpreting a run. Behavioral result semantics stay with the
// runner, while this boundary prevents an ambiguous record from being
// mistaken for a v1 artifact.
func (record RunRecord) Validate() error {
	var problems []string
	if record.Schema != Schema {
		problems = append(problems, fmt.Sprintf("schema must be %s", Schema))
	}
	if strings.TrimSpace(record.RunID) == "" {
		problems = append(problems, "run_id must be non-empty")
	}
	if record.CreatedAt.IsZero() {
		problems = append(problems, "created_at must be present")
	}
	if err := validateContractReference(record.Contract); err != nil {
		problems = append(problems, "contract: "+err.Error())
	}
	if strings.TrimSpace(record.Subject.BaseURL) == "" {
		problems = append(problems, "subject.base_url must be non-empty")
	} else if _, err := parseBaseURL(record.Subject.BaseURL); err != nil {
		problems = append(problems, "subject.base_url: "+err.Error())
	}
	if strings.TrimSpace(record.Subject.Adapter) == "" {
		problems = append(problems, "subject.adapter must be non-empty")
	}
	if record.Oracle != nil {
		if err := validateOracleReference(*record.Oracle); err != nil {
			problems = append(problems, "oracle: "+err.Error())
		}
	}
	if record.Baseline != nil {
		if strings.TrimSpace(record.Baseline.EvidencePath) == "" {
			problems = append(problems, "baseline.evidence_path must be non-empty")
		}
		if strings.TrimSpace(record.Baseline.RunID) == "" {
			problems = append(problems, "baseline.run_id must be non-empty")
		}
		if err := validateContractReference(record.Baseline.Contract); err != nil {
			problems = append(problems, "baseline.contract: "+err.Error())
		}
		if record.Baseline.Oracle != nil {
			if err := validateOracleReference(*record.Baseline.Oracle); err != nil {
				problems = append(problems, "baseline.oracle: "+err.Error())
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid run record:\n- %s", strings.Join(problems, "\n- "))
	}
	return nil
}

func validateContractReference(reference ContractReference) error {
	if strings.TrimSpace(reference.ID) == "" {
		return fmt.Errorf("id must be non-empty")
	}
	if reference.Version < 1 {
		return fmt.Errorf("version must be positive")
	}
	return validateSHA256(reference.SHA256)
}

func validateOracleReference(reference OracleReference) error {
	if reference.Schema != oracle.Schema {
		return fmt.Errorf("schema must be %s", oracle.Schema)
	}
	return validateSHA256(reference.SHA256)
}

func validateSHA256(value string) error {
	if len(value) != sha256.Size*2 {
		return fmt.Errorf("sha256 must be a %d-character hexadecimal digest", sha256.Size*2)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("sha256 must be hexadecimal: %w", err)
	}
	return nil
}
