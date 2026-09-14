package baseline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"ingen/paddock/internal/model"
)

const schema = "paddock.baseline/v1"

type Snapshot struct {
	Schema     string  `json:"schema"`
	ModulePath string  `json:"module_path"`
	Policy     string  `json:"policy,omitempty"`
	Entries    []Entry `json:"entries"`
}

type Entry struct {
	Fingerprint string `json:"fingerprint"`
	RuleID      string `json:"rule_id"`
	Kind        string `json:"kind"`
	Severity    string `json:"severity"`
	From        string `json:"from,omitempty"`
	To          string `json:"to,omitempty"`
	File        string `json:"file,omitempty"`
	Message     string `json:"message,omitempty"`
}

func Build(result *model.Result) Snapshot {
	entries := make([]Entry, 0, len(result.Findings))
	seen := map[string]bool{}
	for _, finding := range result.Findings {
		if finding.Waived {
			continue
		}
		fingerprint := Fingerprint(finding)
		if seen[fingerprint] {
			continue
		}
		seen[fingerprint] = true
		entries = append(entries, Entry{
			Fingerprint: fingerprint,
			RuleID:      finding.RuleID,
			Kind:        finding.Kind,
			Severity:    finding.Severity,
			From:        finding.From,
			To:          finding.To,
			File:        finding.File,
			Message:     finding.Message,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Fingerprint < entries[j].Fingerprint
	})
	return Snapshot{
		Schema:     schema,
		ModulePath: result.ModulePath,
		Policy:     result.Policy,
		Entries:    entries,
	}
}

func Fingerprint(finding *model.Finding) string {
	canonical := strings.Join([]string{
		finding.RuleID,
		finding.Kind,
		finding.From,
		finding.To,
		finding.File,
	}, "\x00")
	digest := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(digest[:])
}

func Load(path string) (Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read baseline: %w", err)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("parse baseline: %w", err)
	}
	if err := snapshot.Validate(); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func Save(path string, snapshot Snapshot) error {
	if err := snapshot.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("encode baseline: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write baseline: %w", err)
	}
	return nil
}

func (s Snapshot) Validate() error {
	if s.Schema != schema {
		return fmt.Errorf("baseline schema must be %s, got %q", schema, s.Schema)
	}
	seen := map[string]bool{}
	for _, entry := range s.Entries {
		if entry.Fingerprint == "" {
			return fmt.Errorf("every baseline entry needs a fingerprint")
		}
		if seen[entry.Fingerprint] {
			return fmt.Errorf("duplicate baseline fingerprint %q", entry.Fingerprint)
		}
		seen[entry.Fingerprint] = true
		if entry.Fingerprint != Fingerprint(&model.Finding{
			RuleID: entry.RuleID,
			Kind:   entry.Kind,
			From:   entry.From,
			To:     entry.To,
			File:   entry.File,
		}) {
			return fmt.Errorf("baseline fingerprint %q does not match its finding identity", entry.Fingerprint)
		}
	}
	return nil
}

func Apply(result *model.Result, snapshot Snapshot, path string) error {
	if snapshot.ModulePath != "" && snapshot.ModulePath != result.ModulePath {
		return fmt.Errorf("baseline module %q does not match source module %q", snapshot.ModulePath, result.ModulePath)
	}
	byFingerprint := make(map[string]Entry, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		byFingerprint[entry.Fingerprint] = entry
	}
	matched := map[string]bool{}
	for _, finding := range result.Findings {
		fingerprint := Fingerprint(finding)
		if _, ok := byFingerprint[fingerprint]; !ok {
			continue
		}
		finding.Baselined = true
		finding.BaselineFingerprint = fingerprint
		matched[fingerprint] = true
	}
	stale := make([]string, 0)
	for _, entry := range snapshot.Entries {
		if !matched[entry.Fingerprint] {
			stale = append(stale, entry.Fingerprint)
		}
	}
	sort.Strings(stale)
	result.Baseline = &model.BaselineSummary{
		Path:    path,
		Entries: len(snapshot.Entries),
		Matched: len(matched),
		Stale:   stale,
	}
	return nil
}
