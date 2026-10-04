package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ingen/sorna/internal/campaign"
	"ingen/sorna/internal/contract"
	"ingen/sorna/internal/lifecycle"
	"ingen/sorna/internal/mutation"
	"ingen/sorna/internal/oracle"
	"ingen/sorna/internal/policy"
	"ingen/sorna/internal/runner"
)

func TestExitCodeSeparatesContractVerdictFromKilledMutation(t *testing.T) {
	if got := exitCodeFor(runner.RunRecord{Verdict: runner.ContractVerdict{Status: "pass"}}); got != 0 {
		t.Fatalf("clean pass exit code = %d, want 0", got)
	}
	if got := exitCodeFor(runner.RunRecord{Verdict: runner.ContractVerdict{Status: "fail"}}); got != 1 {
		t.Fatalf("unexpected contract failure exit code = %d, want 1", got)
	}
	if got := exitCodeFor(runner.RunRecord{
		Verdict:  runner.ContractVerdict{Status: "fail"},
		Mutation: &mutation.Result{Outcome: "killed"},
	}); got != 0 {
		t.Fatalf("killed mutation exit code = %d, want 0", got)
	}
	if got := exitCodeFor(runner.RunRecord{
		Verdict:  runner.ContractVerdict{Status: "pass"},
		Mutation: &mutation.Result{Outcome: "survived"},
	}); got != 1 {
		t.Fatalf("surviving mutation exit code = %d, want 1", got)
	}
}

func TestLifecycleObservationCoverageNamesSamplingBlindSpots(t *testing.T) {
	if got := lifecycleObservationCoverage(nil); got != "unavailable" {
		t.Fatalf("nil coverage = %q, want unavailable", got)
	}
	clean := &lifecycle.AccessTelemetry{
		Status:                      "captured",
		ExecutableSampleCount:       3,
		ExecutableObservationCount:  1,
		ExecutableObservationErrors: 0,
	}
	if got := lifecycleObservationCoverage(clean); got != "periodic-best-effort" {
		t.Fatalf("clean coverage = %q, want periodic-best-effort", got)
	}
	withGap := *clean
	withGap.ExecutableObservationErrors = 1
	if got := lifecycleObservationCoverage(&withGap); got != "periodic-best-effort-with-gaps" {
		t.Fatalf("gap coverage = %q, want periodic-best-effort-with-gaps", got)
	}
	if got := lifecycleObservationLimitation(&lifecycle.AccessTelemetry{ExecutableSamplingIntervalMS: 25}); got != "executable identity was sampled every 25 ms; transitions between samples may be unobserved" {
		t.Fatalf("sampling limitation = %q, want explicit interval limitation", got)
	}
}

func TestParseReplayMatrixCases(t *testing.T) {
	cases, err := parseReplayMatrixCases([]string{
		"baseline=.artifacts/baseline.json|passed|matched",
		"defect=.artifacts/defect.json|failed|drifted",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 || cases[1].ID != "defect" || cases[1].ExpectedCIStatus != "failed" || cases[1].ExpectedReplayState != "drifted" {
		t.Fatalf("parsed replay matrix cases = %+v, want two classified cases", cases)
	}
	if _, err := parseReplayMatrixCases([]string{"broken-case"}); err == nil || !strings.Contains(err.Error(), "expected id=path") {
		t.Fatalf("parseReplayMatrixCases() = %v, want syntax error", err)
	}
}

func TestCampaignEndpointSeparatesLoopbackListenerFromClientURL(t *testing.T) {
	cases := []struct {
		name, host, listener, baseURL string
	}{
		{name: "localhost", host: "localhost", listener: "127.0.0.1:8081", baseURL: "http://localhost:8081"},
		{name: "IPv6", host: "::1", listener: "[::1]:8081", baseURL: "http://[::1]:8081"},
		{name: "other host", host: "example.test", listener: "example.test:8081", baseURL: "http://example.test:8081"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			listener, baseURL := campaignEndpoint(test.host, 8081)
			if listener != test.listener || baseURL != test.baseURL {
				t.Fatalf("campaignEndpoint(%q) = (%q, %q), want (%q, %q)", test.host, listener, baseURL, test.listener, test.baseURL)
			}
		})
	}

	provider := campaign.ProviderManifest{Entries: []campaign.ProviderEntry{{
		MutationID: "m1", Command: "subject", Args: []string{"--listen", "${SORA_ADDR}", "--client-url", "${SORA_URL}"},
	}}}
	listener, baseURL := campaignEndpoint("localhost", 8081)
	prepared, err := provider.Resolve("m1", listener, baseURL)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Command) != 5 || prepared.Command[2] != "127.0.0.1:8081" || prepared.Command[4] != "http://localhost:8081" {
		t.Fatalf("prepared argv = %q; want listener loopback and unchanged client URL", prepared.Command)
	}
}

func TestVerifyCampaignPlanReferenceRejectsExactPlanDrift(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.json")
	plan := verificationPlan()
	exactHash, err := campaign.WriteFile(path, plan)
	if err != nil {
		t.Fatal(err)
	}
	semanticHash, err := campaign.SemanticHash(plan)
	if err != nil {
		t.Fatal(err)
	}
	reference := campaign.PlanReference{Path: path, SHA256: exactHash, SemanticSHA256: semanticHash}
	if err := verifyCampaignPlanReference(reference, "."); err != nil {
		t.Fatalf("verifyCampaignPlanReference() = %v, want initial plan accepted", err)
	}

	plan.Baseline.RunID = "run-drifted"
	if _, err := campaign.WriteFile(path, plan); err != nil {
		t.Fatal(err)
	}
	if err := verifyCampaignPlanReference(reference, "."); err == nil || !strings.Contains(err.Error(), "exact hash") {
		t.Fatalf("verifyCampaignPlanReference() after exact drift = %v, want exact hash mismatch", err)
	}
}

func TestVerifyCampaignPlanReferenceRejectsSemanticPlanDrift(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.json")
	plan := verificationPlan()
	semanticHash, err := campaign.SemanticHash(plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.Mutations[0].Spec.Description = "tampered description"
	exactHash, err := campaign.WriteFile(path, plan)
	if err != nil {
		t.Fatal(err)
	}
	reference := campaign.PlanReference{Path: path, SHA256: exactHash, SemanticSHA256: semanticHash}
	if err := verifyCampaignPlanReference(reference, "."); err == nil || !strings.Contains(err.Error(), "semantic hash") {
		t.Fatalf("verifyCampaignPlanReference() after semantic drift = %v, want semantic hash mismatch", err)
	}
}

func TestOracleFreezeExpectedDigestDriftCreatesNoOutput(t *testing.T) {
	for _, mismatch := range []string{"contract", "policy"} {
		t.Run(mismatch, func(t *testing.T) {
			root, contractPath, policyPath, contractSHA, policySHA := writeOracleFreezeInputs(t)
			outputDir := filepath.Join(root, "frozen")
			args := []string{
				"--root", root,
				"--contract", contractPath,
				"--policy", policyPath,
				"--output-dir", outputDir,
				"--expected-contract-sha256", contractSHA,
				"--expected-policy-sha256", policySHA,
			}
			if mismatch == "contract" {
				args[9] = strings.Repeat("0", 64)
			} else {
				args[11] = strings.Repeat("0", 64)
			}
			if status := freezeOracle(args); status == 0 {
				t.Fatal("freezeOracle accepted expected digest drift")
			}
			if _, err := os.Stat(outputDir); !os.IsNotExist(err) {
				t.Fatalf("output directory stat = %v; expected digest drift must be checked before creating output", err)
			}
		})
	}
}

func TestOracleGeneratePreservesOptionalExpectedDigestBehavior(t *testing.T) {
	root, contractPath, _, contractSHA, _ := writeOracleFreezeInputs(t)
	policySHA := strings.Repeat("a", 64)
	for _, withExpected := range []bool{true, false} {
		outputPath := filepath.Join(root, map[bool]string{true: "expected.json", false: "legacy.json"}[withExpected])
		args := []string{"--contract", contractPath, "--policy-sha256", policySHA, "--output", outputPath}
		if withExpected {
			args = append(args, "--expected-contract-sha256", contractSHA)
		}
		if status := generateOracle(args); status != 0 {
			t.Fatalf("generateOracle(withExpected=%v) status = %d", withExpected, status)
		}
		artifact, err := oracle.LoadFile(outputPath)
		if err != nil || artifact.Contract.SHA256 != contractSHA || artifact.PolicySHA256 != policySHA {
			t.Fatalf("generated artifact = %+v, err %v", artifact, err)
		}
	}

	changed := strings.ReplaceAll(string(mustReadFile(t, contractPath)), `"version":1`, `"version":2`)
	if err := os.WriteFile(contractPath, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	driftPath := filepath.Join(root, "drift.json")
	if status := generateOracle([]string{"--contract", contractPath, "--expected-contract-sha256", contractSHA, "--policy-sha256", policySHA, "--output", driftPath}); status == 0 {
		t.Fatal("generateOracle accepted a contract changed after freeze")
	}
	if _, err := os.Stat(driftPath); !os.IsNotExist(err) {
		t.Fatalf("drift output stat = %v; changed contract must not produce oracle", err)
	}
}

func writeOracleFreezeInputs(t *testing.T) (root, contractPath, policyPath, contractSHA, policySHA string) {
	t.Helper()
	root = t.TempDir()
	contractPath = filepath.Join(root, "contract.json")
	policyPath = filepath.Join(root, "policy.json")
	contractJSON := `{"contract":{"schema":"ingen.contract/v1","id":"freeze-test","version":1,"status":"draft","interface":{"kind":"http-json"},"rules":[{"id":"healthz","strength":"must","subject":"GET /healthz","given":{},"expect":{"status":200}}],"unspecified":[]}}`
	policyJSON := `{"policy":{"schema":"ingen.policy/v1","id":"freeze-test-policy","version":1,"status":"draft","purpose":"oracle generation test","enforcement":"host-enforced","filesystem":{"read":[{"path":".","reason":"contract input"}],"write":[{"path":"frozen","reason":"oracle output"}],"deny":[]},"network":{"mode":"disabled"},"process":{"subject_id":"freeze-test","can_invoke_subject":false}}}`
	if err := os.WriteFile(contractPath, []byte(contractJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policyPath, []byte(policyJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	document, err := contract.LoadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	sealedContract, err := contract.SealAt(document, root)
	if err != nil {
		t.Fatal(err)
	}
	policyDocument, err := policy.LoadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	sealedPolicy, err := policy.Seal(policyDocument)
	if err != nil {
		t.Fatal(err)
	}
	return root, contractPath, policyPath, sealedContract.SHA256, sealedPolicy.SHA256
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}

func verificationPlan() campaign.Plan {
	contractHash := strings.Repeat("a", 64)
	oracle := &runner.OracleReference{Schema: "ingen.oracle/v1", SHA256: strings.Repeat("b", 64)}
	return campaign.Plan{
		Schema:    campaign.Schema,
		Status:    "ready",
		Catalogue: campaign.CatalogueReference{Path: "catalogue.yaml", ID: "catalogue", Version: 1, SHA256: strings.Repeat("c", 64)},
		Contract:  runner.ContractReference{ID: "contract", Version: 1, SHA256: contractHash},
		Oracle:    *oracle,
		Baseline: runner.BaselineReference{
			EvidencePath: "baseline",
			RunID:        "run-baseline",
			Contract:     runner.ContractReference{ID: "contract", Version: 1, SHA256: contractHash},
			Oracle:       oracle,
		},
		OraclePolicySHA256:  strings.Repeat("d", 64),
		SubjectPolicySHA256: strings.Repeat("e", 64),
		Mutations: []campaign.MutationEntry{{
			Sequence: 1,
			Spec: mutation.Spec{
				ID:              "m1",
				Plane:           "implementation",
				Operator:        "test.operator",
				Target:          "GET /",
				Description:     "test mutation",
				Change:          map[string]any{"from": 1, "to": 2},
				ExpectedRuleIDs: []string{"rule-1"},
				Status:          "candidate",
			},
		}},
	}
}
