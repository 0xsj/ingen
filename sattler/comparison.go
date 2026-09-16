// Package sattler contains the first local comparison surface for InGen
// verification artifacts.
package sattler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"ingen/core/ciresult"
)

// Schema identifies the deliberately provisional machine-readable comparison
// report. It is a working surface until Sattler has more producer examples.
const Schema = "ingen.sattler-comparison/v0"

// Change is one observable difference between two valid CI result envelopes.
// Before and After are intentionally small JSON values so the report remains
// useful without exposing producer-owned report semantics.
type Change struct {
	Category string                   `json:"category"`
	Field    string                   `json:"field"`
	Before   any                      `json:"before"`
	After    any                      `json:"after"`
	Identity ArtifactIdentityRelation `json:"identity,omitempty"`
}

// ArtifactSummary contains the envelope fields Sattler can interpret without
// knowing which producer created the nested report.
type ArtifactSummary struct {
	Path       string                      `json:"path,omitempty"`
	Tool       string                      `json:"tool"`
	Kind       string                      `json:"kind"`
	Status     string                      `json:"status"`
	ExitCode   int                         `json:"exit_code"`
	CreatedAt  string                      `json:"created_at"`
	Source     ciresult.Source             `json:"source"`
	Policy     *ciresult.FileRef           `json:"policy,omitempty"`
	PolicyLock *ciresult.FileRef           `json:"policy_lock,omitempty"`
	Graph      *ciresult.FileRef           `json:"graph,omitempty"`
	Baseline   *ciresult.FileRef           `json:"baseline,omitempty"`
	Inputs     map[string]ciresult.FileRef `json:"inputs,omitempty"`
}

// Comparison is a deterministic, producer-neutral comparison of two valid CI
// result envelopes. Compatible means both artifacts describe the same
// producer and result kind; source and input changes remain comparable context.
type Comparison struct {
	Schema               string                      `json:"schema"`
	Compatible           bool                        `json:"compatible"`
	CompatibilityReasons []string                    `json:"compatibility_reasons,omitempty"`
	Before               ArtifactSummary             `json:"before"`
	After                ArtifactSummary             `json:"after"`
	Changes              []Change                    `json:"changes,omitempty"`
	ChangeSummary        ChangeSummary               `json:"change_summary"`
	Warnings             []string                    `json:"warnings,omitempty"`
	MutationCampaign     *MutationCampaignComparison `json:"mutation_campaign,omitempty"`
}

// Compare compares two already validated CI result envelopes.
func Compare(before, after ciresult.Artifact) Comparison {
	report := Comparison{
		Schema: Schema,
		Before: summarize(before),
		After:  summarize(after),
	}
	if before.Tool != after.Tool {
		report.CompatibilityReasons = append(report.CompatibilityReasons, fmt.Sprintf("tool changed from %q to %q", before.Tool, after.Tool))
	}
	if before.Kind != after.Kind {
		report.CompatibilityReasons = append(report.CompatibilityReasons, fmt.Sprintf("kind changed from %q to %q", before.Kind, after.Kind))
	}
	report.Compatible = len(report.CompatibilityReasons) == 0

	add := func(category, field string, oldValue, newValue any) {
		if valuesEqual(oldValue, newValue) {
			return
		}
		report.Changes = append(report.Changes, Change{
			Category: category,
			Field:    field,
			Before:   oldValue,
			After:    newValue,
		})
	}

	add("context", "tool", before.Tool, after.Tool)
	add("context", "kind", before.Kind, after.Kind)
	add("context", "source.root", before.Source.Root, after.Source.Root)
	add("context", "source.module_path", before.Source.ModulePath, after.Source.ModulePath)
	add("verdict", "status", before.Status, after.Status)
	add("verdict", "exit_code", before.ExitCode, after.ExitCode)
	addFileRef := func(category, field string, oldRef, newRef *ciresult.FileRef) {
		if valuesEqual(oldRef, newRef) {
			return
		}
		report.Changes = append(report.Changes, Change{
			Category: category,
			Field:    field,
			Before:   fileRefValue(oldRef),
			After:    fileRefValue(newRef),
			Identity: CompareArtifactIdentity(oldRef, newRef),
		})
	}
	addFileRef("input", "policy", before.Policy, after.Policy)
	addFileRef("input", "policy_lock", before.PolicyLock, after.PolicyLock)
	addFileRef("input", "graph", before.Graph, after.Graph)
	addFileRef("input", "baseline", before.Baseline, after.Baseline)

	inputNames := unionInputNames(before.Inputs, after.Inputs)
	for _, name := range inputNames {
		oldRef, oldOK := before.Inputs[name]
		newRef, newOK := after.Inputs[name]
		var oldRefPointer *ciresult.FileRef
		var newRefPointer *ciresult.FileRef
		if oldOK {
			oldRefCopy := oldRef
			oldRefPointer = &oldRefCopy
		}
		if newOK {
			newRefCopy := newRef
			newRefPointer = &newRefCopy
		}
		addFileRef(inputChangeCategory(name), "inputs."+name, oldRefPointer, newRefPointer)
	}

	add("producer-report", "report", jsonFingerprint(before.Report), jsonFingerprint(after.Report))
	add("producer-report", "explanation", jsonFingerprint(before.Explanation), jsonFingerprint(after.Explanation))
	if campaign, err := CompareMutationCampaigns(before, after); err == nil {
		report.MutationCampaign = campaign
	} else {
		report.Warnings = append(report.Warnings, "mutation campaign detail unavailable: "+err.Error())
	}
	report.ChangeSummary = SummarizeChanges(report.Changes)
	return report
}

// CompareFiles loads and validates two shared CI result envelopes before
// comparing them. The input paths are retained only as report labels.
func CompareFiles(beforePath, afterPath string) (Comparison, error) {
	before, err := ciresult.LoadFile(beforePath)
	if err != nil {
		return Comparison{}, err
	}
	after, err := ciresult.LoadFile(afterPath)
	if err != nil {
		return Comparison{}, err
	}
	report := Compare(before, after)
	report.Before.Path = beforePath
	report.After.Path = afterPath
	return report, nil
}

// WriteJSON writes the provisional machine-readable comparison report.
func WriteJSON(w io.Writer, report Comparison) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

// WriteText writes a compact operator-oriented comparison report.
func WriteText(w io.Writer, report Comparison) error {
	if _, err := fmt.Fprintf(w, "Sattler comparison\n  before: %s (%s/%s, %s)\n  after:  %s (%s/%s, %s)\n  compatible: %t\n", report.Before.Path, report.Before.Tool, report.Before.Kind, report.Before.Status, report.After.Path, report.After.Tool, report.After.Kind, report.After.Status, report.Compatible); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  created: %s -> %s\n", report.Before.CreatedAt, report.After.CreatedAt); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  change summary: %s\n", report.ChangeSummary); err != nil {
		return err
	}
	if len(report.CompatibilityReasons) > 0 {
		if _, err := fmt.Fprintln(w, "  compatibility reasons:"); err != nil {
			return err
		}
		for _, reason := range report.CompatibilityReasons {
			if _, err := fmt.Fprintf(w, "    - %s\n", reason); err != nil {
				return err
			}
		}
	}
	if len(report.Warnings) > 0 {
		if _, err := fmt.Fprintln(w, "  warnings:"); err != nil {
			return err
		}
		for _, warning := range report.Warnings {
			if _, err := fmt.Fprintf(w, "    - %s\n", warning); err != nil {
				return err
			}
		}
	}
	if len(report.Changes) == 0 {
		_, err := fmt.Fprintln(w, "  changes: none observable at the envelope boundary")
		return err
	}
	if _, err := fmt.Fprintln(w, "  changes:"); err != nil {
		return err
	}
	for _, change := range report.Changes {
		identity := ""
		if change.Identity != "" {
			identity = " [" + string(change.Identity) + "]"
		}
		if _, err := fmt.Fprintf(w, "    - %s %s%s: %s -> %s\n", change.Category, change.Field, identity, displayValue(change.Before), displayValue(change.After)); err != nil {
			return err
		}
	}
	if report.MutationCampaign != nil {
		if _, err := fmt.Fprintf(w, "  campaign time: before %s -> %s; after %s -> %s\n", report.MutationCampaign.Before.StartedAt, report.MutationCampaign.Before.FinishedAt, report.MutationCampaign.After.StartedAt, report.MutationCampaign.After.FinishedAt); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  mutation campaign: %d/%d killed -> %d/%d killed\n", report.MutationCampaign.Before.Killed, report.MutationCampaign.Before.Total, report.MutationCampaign.After.Killed, report.MutationCampaign.After.Total); err != nil {
			return err
		}
		for _, change := range report.MutationCampaign.ChangedMutations {
			if _, err := fmt.Fprintf(w, "    - mutation %s: %s -> %s\n", change.MutationID, displayMutationState(change.Before), displayMutationState(change.After)); err != nil {
				return err
			}
		}
	}
	return nil
}

func summarize(artifact ciresult.Artifact) ArtifactSummary {
	return ArtifactSummary{
		Tool:       artifact.Tool,
		Kind:       artifact.Kind,
		Status:     artifact.Status,
		ExitCode:   artifact.ExitCode,
		CreatedAt:  artifact.CreatedAt,
		Source:     artifact.Source,
		Policy:     artifact.Policy,
		PolicyLock: artifact.PolicyLock,
		Graph:      artifact.Graph,
		Baseline:   artifact.Baseline,
		Inputs:     copyInputs(artifact.Inputs),
	}
}

func copyInputs(inputs map[string]ciresult.FileRef) map[string]ciresult.FileRef {
	if len(inputs) == 0 {
		return nil
	}
	copy := make(map[string]ciresult.FileRef, len(inputs))
	for name, ref := range inputs {
		copy[name] = ref
	}
	return copy
}

func fileRefValue(ref *ciresult.FileRef) any {
	if ref == nil {
		return nil
	}
	return *ref
}

func unionInputNames(before, after map[string]ciresult.FileRef) []string {
	seen := make(map[string]bool, len(before)+len(after))
	for name := range before {
		seen[name] = true
	}
	for name := range after {
		seen[name] = true
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func inputChangeCategory(name string) string {
	switch strings.ToLower(name) {
	case "contract", "contract_lock", "contract_schema":
		return "contract"
	case "plan", "mutation_plan", "mutation_catalogue":
		return "plan"
	case "provider", "provider_manifest", "preparation":
		return "provider"
	case "environment", "runtime", "subject_policy":
		return "environment"
	default:
		return "input"
	}
}

func valuesEqual(before, after any) bool {
	beforeJSON, beforeOK := json.Marshal(before)
	afterJSON, afterOK := json.Marshal(after)
	return beforeOK == nil && afterOK == nil && bytes.Equal(beforeJSON, afterJSON)
}

type jsonFingerprintValue struct {
	Present bool   `json:"present"`
	SHA256  string `json:"sha256,omitempty"`
}

func jsonFingerprint(value json.RawMessage) jsonFingerprintValue {
	if len(value) == 0 || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return jsonFingerprintValue{}
	}
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		// Validated envelopes cannot reach this path, but preserving a digest
		// makes the comparison safe if Compare is used directly.
		digest := sha256.Sum256(value)
		return jsonFingerprintValue{Present: true, SHA256: hex.EncodeToString(digest[:])}
	}
	normalized, err := json.Marshal(decoded)
	if err != nil {
		digest := sha256.Sum256(value)
		return jsonFingerprintValue{Present: true, SHA256: hex.EncodeToString(digest[:])}
	}
	digest := sha256.Sum256(normalized)
	return jsonFingerprintValue{Present: true, SHA256: hex.EncodeToString(digest[:])}
}

func displayValue(value any) string {
	if value == nil {
		return "<absent>"
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}

func displayMutationState(state *MutationState) string {
	if state == nil {
		return "<absent>"
	}
	if state.Outcome == "" {
		return state.Status
	}
	return state.Status + "/" + state.Outcome
}
