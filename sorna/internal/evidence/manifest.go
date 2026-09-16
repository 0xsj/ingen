package evidence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"ingen/sorna/internal/policy"
	"ingen/sorna/internal/runner"
)

// LoadManifestFile loads a Sorna evidence manifest using the v1 decoder
// policy. It is deliberately separate from Verify: loading checks the
// manifest's own identity, while Verify additionally checks every referenced
// byte and the relationships between bundle artifacts.
func LoadManifestFile(path string) (Manifest, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode evidence manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Manifest{}, fmt.Errorf("decode evidence manifest: multiple JSON values")
		}
		return Manifest{}, fmt.Errorf("decode evidence manifest trailing data: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// Validate checks the manifest identity needed to interpret a bundle. File
// existence, checksums, and cross-artifact relationships remain Verify's
// responsibility because they require access to the bundle directory.
func (manifest Manifest) Validate() error {
	var problems []string
	if manifest.Schema != Schema {
		problems = append(problems, fmt.Sprintf("schema must be %s", Schema))
	}
	if strings.TrimSpace(manifest.RunID) == "" {
		problems = append(problems, "run_id must be non-empty")
	}
	if manifest.CreatedAt.IsZero() {
		problems = append(problems, "created_at must be present")
	}
	if err := validateManifestContract(manifest.Contract); err != nil {
		problems = append(problems, "contract: "+err.Error())
	}
	if strings.TrimSpace(manifest.Subject.BaseURL) == "" {
		problems = append(problems, "subject.base_url must be non-empty")
	}
	if strings.TrimSpace(manifest.Subject.Adapter) == "" {
		problems = append(problems, "subject.adapter must be non-empty")
	}
	if manifest.Oracle != nil {
		if err := validateManifestOracle(*manifest.Oracle); err != nil {
			problems = append(problems, "oracle: "+err.Error())
		}
	}
	if manifest.Baseline != nil {
		if strings.TrimSpace(manifest.Baseline.EvidencePath) == "" {
			problems = append(problems, "baseline.evidence_path must be non-empty")
		}
		if strings.TrimSpace(manifest.Baseline.RunID) == "" {
			problems = append(problems, "baseline.run_id must be non-empty")
		}
		if err := validateManifestContract(manifest.Baseline.Contract); err != nil {
			problems = append(problems, "baseline.contract: "+err.Error())
		}
		if manifest.Baseline.Oracle != nil {
			if err := validateManifestOracle(*manifest.Baseline.Oracle); err != nil {
				problems = append(problems, "baseline.oracle: "+err.Error())
			}
		}
	}
	for name, reference := range map[string]*policy.Reference{
		"policy":         manifest.Policy,
		"subject_policy": manifest.SubjectPolicy,
	} {
		if reference == nil {
			continue
		}
		if err := validatePolicyReference(*reference); err != nil {
			problems = append(problems, name+": "+err.Error())
		}
	}
	if len(manifest.ArtifactsSHA256) == 0 {
		problems = append(problems, "artifacts_sha256 must contain at least one artifact")
	} else {
		for path, digest := range manifest.ArtifactsSHA256 {
			if _, err := safeRelativePath(path); err != nil {
				problems = append(problems, "artifacts_sha256: "+err.Error())
			}
			if err := validateSHA256Digest(digest); err != nil {
				problems = append(problems, "artifacts_sha256["+path+"]: "+err.Error())
			}
		}
	}
	if manifest.Campaign != nil {
		if err := validateCampaignProvenance(*manifest.Campaign, manifest.RunID); err != nil {
			problems = append(problems, "campaign: "+err.Error())
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid evidence manifest:\n- %s", strings.Join(problems, "\n- "))
	}
	return nil
}

func validateManifestContract(reference runner.ContractReference) error {
	if strings.TrimSpace(reference.ID) == "" {
		return fmt.Errorf("id must be non-empty")
	}
	if reference.Version < 1 {
		return fmt.Errorf("version must be positive")
	}
	return validateSHA256Digest(reference.SHA256)
}

func validateManifestOracle(reference runner.OracleReference) error {
	if reference.Schema != "ingen.oracle/v1" {
		return fmt.Errorf("schema must be ingen.oracle/v1")
	}
	return validateSHA256Digest(reference.SHA256)
}

func validatePolicyReference(reference policy.Reference) error {
	if strings.TrimSpace(reference.ID) == "" {
		return fmt.Errorf("id must be non-empty")
	}
	if reference.Version < 1 {
		return fmt.Errorf("version must be positive")
	}
	return validateSHA256Digest(reference.SHA256)
}

func validateSHA256Digest(value string) error {
	if len(value) != sha256.Size*2 {
		return fmt.Errorf("sha256 must be a %d-character hexadecimal digest", sha256.Size*2)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("sha256 must be hexadecimal: %w", err)
	}
	return nil
}
