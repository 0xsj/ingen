package sattler

import (
	"encoding/json"
	"fmt"
	"sort"

	"ingen/core/ciresult"
)

// MutationCampaignSchema is the Sorna report schema understood by this
// adapter. The adapter reads the report; Sorna remains its authority.
const MutationCampaignSchema = "ingen.mutation-campaign-result/v1"

// MutationCampaignSummary is the producer-owned campaign information Sattler
// can safely summarize without re-running or revalidating the campaign.
type MutationCampaignSummary struct {
	Schema             string `json:"schema"`
	Status             string `json:"status"`
	StartedAt          string `json:"started_at"`
	FinishedAt         string `json:"finished_at"`
	PlanSHA256         string `json:"plan_sha256"`
	SemanticPlanSHA256 string `json:"semantic_plan_sha256,omitempty"`
	Total              int    `json:"total"`
	Killed             int    `json:"killed"`
	Survived           int    `json:"survived"`
	Inconclusive       int    `json:"inconclusive"`
	Other              int    `json:"other"`
	Errors             int    `json:"errors"`
}

// MutationState is the compact producer-owned state for one mutation entry.
type MutationState struct {
	Status  string `json:"status"`
	Outcome string `json:"outcome,omitempty"`
}

// MutationOutcomeChange identifies a mutation whose recorded state changed.
type MutationOutcomeChange struct {
	MutationID string         `json:"mutation_id"`
	Before     *MutationState `json:"before,omitempty"`
	After      *MutationState `json:"after,omitempty"`
}

// MutationCampaignComparison is an optional producer-aware extension to the
// generic envelope comparison.
type MutationCampaignComparison struct {
	Before           MutationCampaignSummary `json:"before"`
	After            MutationCampaignSummary `json:"after"`
	ChangedMutations []MutationOutcomeChange `json:"changed_mutations,omitempty"`
}

type mutationCampaignReport struct {
	Schema     string `json:"schema"`
	Status     string `json:"status"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
	Plan       struct {
		SHA256         string `json:"sha256"`
		SemanticSHA256 string `json:"semantic_sha256"`
	} `json:"plan"`
	Summary struct {
		Total        int `json:"total"`
		Killed       int `json:"killed"`
		Survived     int `json:"survived"`
		Inconclusive int `json:"inconclusive"`
		Other        int `json:"other"`
		Errors       int `json:"errors"`
	} `json:"summary"`
	Entries []struct {
		MutationID string `json:"mutation_id"`
		Status     string `json:"status"`
		Outcome    string `json:"outcome"`
	} `json:"entries"`
}

type mutationCampaignData struct {
	Summary MutationCampaignSummary
	States  map[string]MutationState
}

// ExtractMutationCampaign recognizes the Sorna mutation-campaign report in a
// shared CI envelope. It returns recognized=false for other producer results.
func ExtractMutationCampaign(artifact ciresult.Artifact) (summary MutationCampaignSummary, recognized bool, err error) {
	data, recognized, err := extractMutationCampaign(artifact)
	if err != nil || !recognized {
		return MutationCampaignSummary{}, recognized, err
	}
	return data.Summary, true, nil
}

// CompareMutationCampaigns compares the producer-owned mutation summary when
// both envelopes contain the recognized Sorna campaign report.
func CompareMutationCampaigns(before, after ciresult.Artifact) (*MutationCampaignComparison, error) {
	beforeData, beforeRecognized, err := extractMutationCampaign(before)
	if err != nil {
		return nil, err
	}
	afterData, afterRecognized, err := extractMutationCampaign(after)
	if err != nil {
		return nil, err
	}
	if !beforeRecognized && !afterRecognized {
		return nil, nil
	}
	if beforeRecognized != afterRecognized {
		return nil, fmt.Errorf("mutation campaign comparison needs both envelopes to contain %s", MutationCampaignSchema)
	}

	comparison := &MutationCampaignComparison{
		Before: beforeData.Summary,
		After:  afterData.Summary,
	}
	names := make([]string, 0, len(beforeData.States)+len(afterData.States))
	seen := make(map[string]bool, len(beforeData.States)+len(afterData.States))
	for name := range beforeData.States {
		seen[name] = true
	}
	for name := range afterData.States {
		seen[name] = true
	}
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		beforeState, beforeOK := beforeData.States[name]
		afterState, afterOK := afterData.States[name]
		if beforeOK && afterOK && beforeState == afterState {
			continue
		}
		change := MutationOutcomeChange{MutationID: name}
		if beforeOK {
			state := beforeState
			change.Before = &state
		}
		if afterOK {
			state := afterState
			change.After = &state
		}
		comparison.ChangedMutations = append(comparison.ChangedMutations, change)
	}
	return comparison, nil
}

func extractMutationCampaign(artifact ciresult.Artifact) (mutationCampaignData, bool, error) {
	if artifact.Tool != "sorna" || artifact.Kind != "mutation-campaign" {
		return mutationCampaignData{}, false, nil
	}
	var report mutationCampaignReport
	if err := json.Unmarshal(artifact.Report, &report); err != nil {
		return mutationCampaignData{}, true, fmt.Errorf("decode Sorna mutation campaign report: %w", err)
	}
	if report.Schema != MutationCampaignSchema {
		return mutationCampaignData{}, true, fmt.Errorf("Sorna mutation campaign report schema must be %s, got %q", MutationCampaignSchema, report.Schema)
	}
	data := mutationCampaignData{
		Summary: MutationCampaignSummary{
			Schema:             report.Schema,
			Status:             report.Status,
			StartedAt:          report.StartedAt,
			FinishedAt:         report.FinishedAt,
			PlanSHA256:         report.Plan.SHA256,
			SemanticPlanSHA256: report.Plan.SemanticSHA256,
			Total:              report.Summary.Total,
			Killed:             report.Summary.Killed,
			Survived:           report.Summary.Survived,
			Inconclusive:       report.Summary.Inconclusive,
			Other:              report.Summary.Other,
			Errors:             report.Summary.Errors,
		},
		States: make(map[string]MutationState, len(report.Entries)),
	}
	for _, entry := range report.Entries {
		if entry.MutationID == "" {
			continue
		}
		data.States[entry.MutationID] = MutationState{Status: entry.Status, Outcome: entry.Outcome}
	}
	return data, true, nil
}
