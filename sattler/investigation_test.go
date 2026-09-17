package sattler

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewInvestigationReportPreservesFindingsEvidenceAndUnknowns(t *testing.T) {
	bundle, err := CompareBundleFile(filepath.Join("testdata", "cross-artifact", "comparison.json"))
	if err != nil {
		t.Fatal(err)
	}

	report := NewInvestigationReport(bundle)
	if report.Schema != InvestigationSchema || report.Compatible {
		t.Fatalf("investigation report = %+v, want schema and bundle compatibility preserved", report)
	}
	if report.Notice == "" || !strings.Contains(strings.ToLower(report.Notice), "not regression") || !strings.Contains(strings.ToLower(report.Notice), "causation") {
		t.Fatalf("investigation notice = %q, want explicit neutral caveat", report.Notice)
	}
	if len(report.Findings) != 2 {
		t.Fatalf("findings = %+v, want one rule and one raw Sorna mutation finding", report.Findings)
	}
	var ruleFinding, mutationFinding *InvestigationFinding
	for index := range report.Findings {
		switch report.Findings[index].Kind {
		case "rule-change":
			ruleFinding = &report.Findings[index]
		case "mutation-change":
			mutationFinding = &report.Findings[index]
		}
	}
	if ruleFinding == nil || ruleFinding.Subject != "document.create.accepted" || ruleFinding.Before != "pass" || ruleFinding.After != "fail" || ruleFinding.Confidence != InvestigationConfidenceDirect {
		t.Fatalf("rule finding = %+v, want direct pass-to-fail observation", ruleFinding)
	}
	if mutationFinding == nil || mutationFinding.Subject != "mut-alpha" || mutationFinding.Confidence != InvestigationConfidenceDirect {
		t.Fatalf("mutation finding = %+v, want direct Sorna mutation observation", mutationFinding)
	}
	if len(ruleFinding.RelatedEvidenceIDs) == 0 || len(mutationFinding.RelatedEvidenceIDs) == 0 {
		t.Fatalf("findings = %+v, want co-observed evidence references", report.Findings)
	}
	if len(report.Evidence) != 24 {
		t.Fatalf("evidence changes = %d, want 24 non-rule/non-mutation adapter changes", len(report.Evidence))
	}
	var contractEvidence, reportEvidence *InvestigationEvidenceChange
	for index := range report.Evidence {
		switch report.Evidence[index].ID {
		case "ci_result.contract.inputs.contract":
			contractEvidence = &report.Evidence[index]
		case "ci_result.producer-report.report":
			reportEvidence = &report.Evidence[index]
		}
	}
	if contractEvidence == nil || contractEvidence.Identity != ArtifactIdentityReplaced || contractEvidence.Confidence != InvestigationConfidenceDirect {
		t.Fatalf("contract evidence = %+v, want replaced direct evidence", contractEvidence)
	}
	if reportEvidence == nil {
		t.Fatalf("evidence = %+v, want producer report change retained as evidence", report.Evidence)
	}
	if len(report.Unknowns) != 1 || report.Unknowns[0].ID != "missing-mutation-campaign-detail" || report.Unknowns[0].Confidence != InvestigationConfidenceUnknown {
		t.Fatalf("unknowns = %+v, want explicit missing campaign detail", report.Unknowns)
	}

	var first, second bytes.Buffer
	if err := WriteInvestigationJSON(&first, report); err != nil {
		t.Fatal(err)
	}
	if err := WriteInvestigationJSON(&second, NewInvestigationReport(bundle)); err != nil {
		t.Fatal(err)
	}
	if first.String() != second.String() {
		t.Fatalf("investigation JSON is not deterministic:\nfirst=%s\nsecond=%s", first.String(), second.String())
	}

	var document InvestigationReport
	if err := json.Unmarshal(first.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Findings) != 2 || len(document.Evidence) != 24 || len(document.Unknowns) != 1 {
		t.Fatalf("decoded investigation document = %+v, want stable projections", document)
	}

	var textOutput bytes.Buffer
	if err := WriteInvestigationText(&textOutput, report); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"Sattler investigation", "rule-change \"document.create.accepted\"", "mutation-change \"mut-alpha\"", "ci_result.contract.inputs.contract [replaced]", "unknowns:", "missing-mutation-campaign-detail"} {
		if !strings.Contains(textOutput.String(), fragment) {
			t.Fatalf("investigation text = %q, missing %q", textOutput.String(), fragment)
		}
	}
}

func TestNewInvestigationReportUsesUnknownConfidenceWithoutSourcePaths(t *testing.T) {
	report := NewInvestigationReport(BundleComparison{
		SornaRun: &SornaRunComparison{
			Before: SornaRunSummary{Mutation: &SornaRunMutationSummary{ID: "mutation-a", Outcome: "killed"}},
			After:  SornaRunSummary{Mutation: &SornaRunMutationSummary{ID: "mutation-a", Outcome: "survived"}},
		},
	})
	if len(report.Findings) != 1 || report.Findings[0].Confidence != InvestigationConfidenceUnknown {
		t.Fatalf("findings = %+v, want unknown confidence without source paths", report.Findings)
	}
}
