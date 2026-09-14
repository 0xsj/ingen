package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"time"

	"ingen/sorna/internal/contract"
	"ingen/sorna/internal/evidence"
	"ingen/sorna/internal/lifecycle"
	"ingen/sorna/internal/mutation"
	"ingen/sorna/internal/oracle"
	"ingen/sorna/internal/policy"
	"ingen/sorna/internal/runner"
	"ingen/sorna/internal/sandbox"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}

	switch args[0] {
	case "contract":
		return contractCommand(args[1:])
	case "evidence":
		return evidenceCommand(args[1:])
	case "policy":
		return policyCommand(args[1:])
	case "sandbox":
		return sandboxCommand(args[1:])
	case "oracle":
		return oracleCommand(args[1:])
	case "run":
		return runSubject(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", args[0])
		usage()
		return 2
	}
}

func sandboxCommand(args []string) int {
	if len(args) == 0 || args[0] != "exec" {
		fmt.Fprintln(os.Stderr, "usage: sorna sandbox exec --policy <path> [--root <dir>] -- <command> [args...]")
		return 2
	}
	flags := flag.NewFlagSet("sandbox exec", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	policyPath := flags.String("policy", "", "path to an isolation policy")
	rootDir := flags.String("root", ".", "sandbox root used to resolve relative policy paths")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	command := flags.Args()
	if *policyPath == "" || len(command) == 0 {
		fmt.Fprintln(os.Stderr, "usage: sorna sandbox exec --policy <path> [--root <dir>] -- <command> [args...]")
		return 2
	}

	document, err := policy.LoadFile(*policyPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	sealed, err := policy.Seal(document)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	prepared, err := sandbox.Prepare(command, *rootDir, sealed)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "sandbox: backend=%s enforcement=%s policy=%s\n", prepared.Backend, prepared.Enforcement, prepared.PolicySHA256)
	process := osexec.Command(prepared.Command[0], prepared.Command[1:]...)
	process.Dir = *rootDir
	process.Stdin = os.Stdin
	process.Stdout = os.Stdout
	process.Stderr = os.Stderr
	if err := process.Run(); err != nil {
		var exitErr *osexec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func oracleCommand(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: sorna oracle freeze --contract <path> --policy <path> [--root <dir>] [--output-dir <dir>]")
		return 2
	}
	switch args[0] {
	case "freeze":
		return freezeOracle(args[1:])
	case "generate":
		return generateOracle(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "unknown oracle command:", args[0])
		fmt.Fprintln(os.Stderr, "usage: sorna oracle freeze --contract <path> --policy <path> [--root <dir>] [--output-dir <dir>]")
		return 2
	}
}

func freezeOracle(args []string) int {
	flags := flag.NewFlagSet("oracle freeze", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	contractPath := flags.String("contract", "", "path to the contract, resolved from root")
	policyPath := flags.String("policy", "", "path to the isolation policy, resolved from root")
	rootDir := flags.String("root", ".", "root used to resolve relative paths")
	outputDir := flags.String("output-dir", ".artifacts/document-pipeline-oracle", "directory for frozen oracle evidence")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *contractPath == "" || *policyPath == "" {
		fmt.Fprintln(os.Stderr, "usage: sorna oracle freeze --contract <path> --policy <path> [--root <dir>] [--output-dir <dir>]")
		return 2
	}

	rootAbs, err := filepath.Abs(*rootDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolve oracle root:", err)
		return 1
	}
	resolvedContractPath := pathFromRoot(rootAbs, *contractPath)
	resolvedPolicyPath := pathFromRoot(rootAbs, *policyPath)
	resolvedOutputDir := pathFromRoot(rootAbs, *outputDir)
	if err := os.MkdirAll(resolvedOutputDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "create oracle output directory:", err)
		return 1
	}

	contractDocument, err := contract.LoadFile(resolvedContractPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	sealedContract, err := contract.SealAt(contractDocument, filepath.Dir(resolvedContractPath))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	policyDocument, err := policy.LoadFile(resolvedPolicyPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	sealedPolicy, err := policy.Seal(policyDocument)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	oraclePath := filepath.Join(resolvedOutputDir, "oracle.json")
	childCommand := []string{os.Args[0], "oracle", "generate", "--contract", resolvedContractPath, "--policy-sha256", sealedPolicy.SHA256, "--output", oraclePath}
	prepared, err := sandbox.Prepare(childCommand, *rootDir, sealedPolicy)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !sandbox.PathCovered(prepared.AllowedReads, resolvedContractPath) {
		fmt.Fprintln(os.Stderr, "oracle contract path is outside the policy read roots")
		return 1
	}
	if !sandbox.PathCovered(prepared.AllowedWrites, oraclePath) {
		fmt.Fprintln(os.Stderr, "oracle output path is outside the policy write roots")
		return 1
	}

	executionID := "oracle-" + fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	startedAt := time.Now().UTC()
	execution := evidence.OracleExecution{
		ExecutionID:   executionID,
		Mode:          "sandboxed-process",
		Command:       append([]string(nil), childCommand...),
		WorkingDir:    rootAbs,
		Backend:       prepared.Backend,
		Enforcement:   prepared.Enforcement,
		PolicySHA256:  prepared.PolicySHA256,
		AllowedReads:  append([]string(nil), prepared.AllowedReads...),
		AllowedWrites: append([]string(nil), prepared.AllowedWrites...),
		DeniedPaths:   append([]string(nil), prepared.DeniedPaths...),
		StartedAt:     startedAt,
		Outcome:       "running",
		Events: []evidence.OracleExecutionEvent{{
			EventID:     "evt-0001",
			ExecutionID: executionID,
			Sequence:    1,
			Timestamp:   startedAt,
			Actor:       "sorna",
			Kind:        "oracle.sandbox.prepared",
			Payload: map[string]any{
				"backend":       prepared.Backend,
				"enforcement":   prepared.Enforcement,
				"policy_sha256": prepared.PolicySHA256,
			},
		}},
	}
	process := osexec.Command(prepared.Command[0], prepared.Command[1:]...)
	process.Dir = *rootDir
	process.Stdin = os.Stdin
	process.Stdout = os.Stdout
	process.Stderr = os.Stderr
	execution.Events = append(execution.Events, evidence.OracleExecutionEvent{
		EventID:     "evt-0002",
		ExecutionID: executionID,
		Sequence:    2,
		Timestamp:   startedAt,
		Actor:       "sorna",
		Kind:        "oracle.process.started",
		Payload:     map[string]any{},
	})
	if err := process.Run(); err != nil {
		execution.Outcome = "failed"
		fmt.Fprintln(os.Stderr, "oracle generation failed:", err)
		return 1
	}
	completedAt := time.Now().UTC()
	execution.CompletedAt = completedAt
	execution.Outcome = "completed"
	execution.Events = append(execution.Events, evidence.OracleExecutionEvent{
		EventID:     "evt-0003",
		ExecutionID: executionID,
		Sequence:    3,
		Timestamp:   completedAt,
		Actor:       "sorna",
		Kind:        "oracle.process.completed",
		Payload:     map[string]any{},
	})

	artifact, err := oracle.LoadFile(oraclePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load generated oracle:", err)
		return 1
	}
	if artifact.Contract.SHA256 != sealedContract.SHA256 || artifact.PolicySHA256 != sealedPolicy.SHA256 {
		fmt.Fprintln(os.Stderr, "generated oracle is not bound to the requested contract and policy")
		return 1
	}
	bundle, err := evidence.WriteOracleBundle(resolvedOutputDir, artifact, execution, sealedPolicy)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("oracle: %s\nmanifest: %s\ncontract: %s (%s)\npolicy: %s\nexecution: %s (%s)\n", bundle.OraclePath, bundle.ManifestPath, artifact.Contract.SHA256, artifact.Contract.ID, artifact.PolicySHA256, execution.Backend, execution.Outcome)
	return 0
}

func generateOracle(args []string) int {
	flags := flag.NewFlagSet("oracle generate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	contractPath := flags.String("contract", "", "path to the contract")
	policyHash := flags.String("policy-sha256", "", "sealed policy SHA-256 hash")
	outputPath := flags.String("output", "", "path for the frozen oracle")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *contractPath == "" || *policyHash == "" || *outputPath == "" {
		fmt.Fprintln(os.Stderr, "usage: sorna oracle generate --contract <path> --policy-sha256 <hash> --output <path>")
		return 2
	}
	document, err := contract.LoadFile(*contractPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	sealed, err := contract.SealAt(document, filepath.Dir(*contractPath))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	artifact, err := oracle.Generate(sealed, *policyHash)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if _, err := oracle.WriteFile(*outputPath, artifact); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "oracle generated: %s (%d cases)\n", *outputPath, len(artifact.Cases))
	return 0
}

func pathFromRoot(root, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(root, path))
}

func evidenceCommand(args []string) int {
	if len(args) != 2 || args[0] != "verify" {
		fmt.Fprintln(os.Stderr, "usage: sorna evidence verify <directory>")
		return 2
	}
	if err := evidence.Verify(args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("verified:", args[1])
	return 0
}

func policyCommand(args []string) int {
	if len(args) < 1 {
		usage()
		return 2
	}
	switch args[0] {
	case "validate":
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "usage: sorna policy validate <path>")
			return 2
		}
		if _, err := policy.LoadFile(args[1]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("valid:", args[1])
		return 0
	case "seal":
		return sealPolicy(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "unknown policy command:", args[0])
		usage()
		return 2
	}
}

func sealPolicy(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: sorna policy seal <path> --output-dir <dir>")
		return 2
	}
	flags := flag.NewFlagSet("policy seal", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	outputDir := flags.String("output-dir", "", "directory for sealed policy artifacts")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if *outputDir == "" {
		fmt.Fprintln(os.Stderr, "--output-dir is required")
		return 2
	}
	sealed, err := policy.SealFile(args[0], *outputDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("sealed:", sealed.SHA256)
	return 0
}

func contractCommand(args []string) int {
	if len(args) < 1 {
		usage()
		return 2
	}

	switch args[0] {
	case "validate":
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "usage: sorna contract validate <path>")
			return 2
		}
		if _, err := contract.LoadFile(args[1]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("valid:", args[1])
		return 0
	case "seal":
		return seal(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "unknown contract command:", args[0])
		usage()
		return 2
	}
}

func seal(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: sorna contract seal <path> --output-dir <dir>")
		return 2
	}
	flags := flag.NewFlagSet("seal", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	outputDir := flags.String("output-dir", "", "directory for sealed artifacts")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if *outputDir == "" {
		fmt.Fprintln(os.Stderr, "--output-dir is required")
		return 2
	}
	sealed, err := contract.SealFile(args[0], *outputDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("sealed:", sealed.SHA256)
	return 0
}

func runSubject(args []string) int {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	contractPath := flags.String("contract", "", "path to the contract")
	oraclePath := flags.String("oracle", "", "path to a canonical frozen oracle artifact")
	policyPath := flags.String("policy", "", "path to an isolation policy")
	baseURL := flags.String("base-url", "", "absolute URL of the running subject")
	outputDir := flags.String("output-dir", ".artifacts/document-pipeline-run", "directory for the run record")
	variant := flags.String("subject-variant", "clean-baseline", "label for the subject variant")
	subjectCommand := flags.String("subject-command", "", "executable for a subject Sorna should manage")
	subjectDir := flags.String("subject-dir", "", "working directory for the managed subject")
	readyPath := flags.String("ready-path", "/healthz", "HTTP path that must return 2xx before the run")
	startupTimeout := flags.Duration("startup-timeout", 10*time.Second, "maximum time to wait for subject readiness")
	shutdownTimeout := flags.Duration("shutdown-timeout", 5*time.Second, "maximum time to wait for graceful subject shutdown")
	var subjectArgs stringList
	flags.Var(&subjectArgs, "subject-arg", "argument for the managed subject; may be repeated")
	mutationID := flags.String("mutation-id", "", "identity of the mutation being evaluated")
	mutationPlane := flags.String("mutation-plane", "behavior", "mutation plane")
	mutationDescription := flags.String("mutation-description", "", "description of the mutation")
	expectedRule := flags.String("expected-rule", "", "rule ID expected to observe the mutation")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if (*contractPath == "" && *oraclePath == "") || *baseURL == "" {
		fmt.Fprintln(os.Stderr, "usage: sorna run [--oracle <path> | --contract <path>] [--policy <path>] --base-url <url> [--subject-command <executable> --subject-arg <arg> ...] [--output-dir <dir>]")
		return 2
	}
	if *contractPath != "" && *oraclePath != "" {
		fmt.Fprintln(os.Stderr, "--oracle and --contract are mutually exclusive; run from the frozen oracle or use the legacy contract path")
		return 2
	}
	if *subjectCommand == "" && len(subjectArgs) > 0 {
		fmt.Fprintln(os.Stderr, "--subject-arg requires --subject-command")
		return 2
	}

	var sealed contract.Sealed
	var oracleArtifact oracle.Artifact
	var err error
	usingOracle := *oraclePath != ""
	if usingOracle {
		oracleArtifact, err = oracle.LoadFile(*oraclePath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	} else {
		document, loadErr := contract.LoadFile(*contractPath)
		if loadErr != nil {
			fmt.Fprintln(os.Stderr, loadErr)
			return 1
		}
		sealed, err = contract.Seal(document)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	var sealedPolicy *policy.Sealed
	if *policyPath != "" {
		policyDocument, err := policy.LoadFile(*policyPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		sealedValue, err := policy.Seal(policyDocument)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		sealedPolicy = &sealedValue
	}
	if usingOracle && sealedPolicy != nil && oracleArtifact.PolicySHA256 != sealedPolicy.SHA256 {
		fmt.Fprintln(os.Stderr, "oracle policy hash does not match the supplied policy")
		return 1
	}
	var mutationSpec *mutation.Spec
	if *mutationID != "" {
		if *expectedRule == "" {
			fmt.Fprintln(os.Stderr, "--expected-rule is required when --mutation-id is provided")
			return 2
		}
		mutationSpec = &mutation.Spec{
			ID:              *mutationID,
			Plane:           *mutationPlane,
			Description:     *mutationDescription,
			ExpectedRuleIDs: []string{*expectedRule},
		}
	}

	var managedSubject *lifecycle.Process
	lifecycleRecord := lifecycle.External(*baseURL, nil)
	if *subjectCommand != "" {
		command := append([]string{*subjectCommand}, subjectArgs...)
		managedSubject, err = lifecycle.Start(context.Background(), lifecycle.Config{
			Command:         command,
			Dir:             *subjectDir,
			BaseURL:         *baseURL,
			ReadyPath:       *readyPath,
			StartupTimeout:  *startupTimeout,
			ShutdownTimeout: *shutdownTimeout,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		lifecycleRecord = managedSubject.Record()
	}

	var record runner.RunRecord
	var runErr error
	config := runner.Config{
		BaseURL:   *baseURL,
		Variant:   *variant,
		Mutation:  mutationSpec,
		Lifecycle: &lifecycleRecord,
	}
	if usingOracle {
		record, runErr = runner.ExecuteOracle(context.Background(), oracleArtifact, config)
	} else {
		record, runErr = runner.Execute(context.Background(), sealed, config)
	}
	var stopErr error
	if managedSubject != nil {
		stopErr = managedSubject.Close(context.Background())
		lifecycleRecord = managedSubject.Record()
	}
	if runErr != nil {
		fmt.Fprintln(os.Stderr, runErr)
		return 1
	}
	if stopErr != nil {
		fmt.Fprintln(os.Stderr, stopErr)
		return 1
	}
	record.Lifecycle = &lifecycleRecord
	bundle, err := evidence.WriteBundle(*outputDir, record, sealedPolicy)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	path := bundle.RunPath
	fmt.Printf("run: %s\ncontract: %s (%s)\nsummary: %d passed, %d failed, %d errors, %d inconclusive, %d skipped\n", path, record.Verdict.Status, record.Verdict.Reason, record.Summary.Passed, record.Summary.Failed, record.Summary.Errors, record.Summary.Inconclusive, record.Summary.Skipped)
	if record.Oracle != nil {
		fmt.Printf("oracle: %s\n", record.Oracle.SHA256)
	}
	if record.Mutation != nil {
		fmt.Printf("mutation: %s -> %s\n", record.Mutation.Spec.ID, record.Mutation.Outcome)
	}
	return exitCodeFor(record)
}

func exitCodeFor(record runner.RunRecord) int {
	if record.Mutation != nil {
		if record.Mutation.Outcome == "killed" {
			return 0
		}
		return 1
	}
	if record.Verdict.Status != "pass" {
		return 1
	}
	return 0
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  sorna contract validate <path>")
	fmt.Fprintln(os.Stderr, "  sorna contract seal <path> --output-dir <dir>")
	fmt.Fprintln(os.Stderr, "  sorna policy validate <path>")
	fmt.Fprintln(os.Stderr, "  sorna policy seal <path> --output-dir <dir>")
	fmt.Fprintln(os.Stderr, "  sorna sandbox exec --policy <path> [--root <dir>] -- <command> [args...]")
	fmt.Fprintln(os.Stderr, "  sorna oracle freeze --contract <path> --policy <path> [--root <dir>] [--output-dir <dir>]")
	fmt.Fprintln(os.Stderr, "  sorna evidence verify <directory>")
	fmt.Fprintln(os.Stderr, "  sorna run [--oracle <path> | --contract <path>] [--policy <path>] --base-url <url> [--subject-command <executable> --subject-arg <arg> ...] [--output-dir <dir>]")
}

type stringList []string

func (list *stringList) String() string {
	return fmt.Sprint([]string(*list))
}

func (list *stringList) Set(value string) error {
	*list = append(*list, value)
	return nil
}
