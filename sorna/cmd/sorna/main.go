package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ingen/core/ciresult"
	"ingen/sorna/internal/campaign"
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
	case "gate":
		return gateCommand(args[1:])
	case "policy":
		return policyCommand(args[1:])
	case "sandbox":
		return sandboxCommand(args[1:])
	case "oracle":
		return oracleCommand(args[1:])
	case "mutation":
		return mutationCommand(args[1:])
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
	contractID, _ := sealedContract.Document.Contract["id"].(string)
	policySubjectID, err := policy.SubjectID(sealedPolicy.Document)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if policySubjectID != contractID {
		fmt.Fprintf(os.Stderr, "oracle policy subject ID %q does not match contract ID %q\n", policySubjectID, contractID)
		return 1
	}

	oraclePath := filepath.Join(resolvedOutputDir, "oracle.json")
	childCommand := []string{os.Args[0], "oracle", "generate", "--contract", resolvedContractPath, "--policy-sha256", sealedPolicy.SHA256, "--output", oraclePath, "--start-gate-fd", "3"}
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
		ExecutionID:      executionID,
		Mode:             "sandboxed-process",
		Command:          append([]string(nil), childCommand...),
		WorkingDir:       rootAbs,
		Backend:          prepared.Backend,
		Enforcement:      prepared.Enforcement,
		PolicySHA256:     prepared.PolicySHA256,
		SubjectID:        prepared.SubjectID,
		ExecutablePath:   prepared.ExecutablePath,
		ExecutableSHA256: prepared.ExecutableSHA256,
		CanInvokeSubject: prepared.CanInvokeSubject,
		AllowedTools:     append([]string(nil), prepared.AllowedTools...),
		AllowedReads:     append([]string(nil), prepared.AllowedReads...),
		AllowedWrites:    append([]string(nil), prepared.AllowedWrites...),
		DeniedPaths:      append([]string(nil), prepared.DeniedPaths...),
		StartedAt:        startedAt,
		Outcome:          "running",
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
	gateReader, gateWriter, err := os.Pipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "create oracle start gate:", err)
		return 1
	}
	process.ExtraFiles = []*os.File{gateReader}
	accessCapture, accessErr := sandbox.StartAccessCapture(context.Background())
	if accessErr != nil {
		execution.Access = evidence.AccessTelemetry{
			Status: "unavailable",
			Source: "macos-unified-log",
			Reason: accessErr.Error(),
		}
	} else {
		execution.Access = evidence.AccessTelemetry{
			Status: "collecting",
			Source: "macos-unified-log",
		}
	}
	execution.Events = append(execution.Events, evidence.OracleExecutionEvent{
		EventID:     "evt-0002",
		ExecutionID: executionID,
		Sequence:    2,
		Timestamp:   startedAt,
		Actor:       "sorna",
		Kind:        "oracle.process.started",
		Payload:     map[string]any{},
	})
	if err := process.Start(); err != nil {
		_ = gateReader.Close()
		_ = gateWriter.Close()
		if accessCapture != nil {
			_, _ = accessCapture.Stop(0)
		}
		execution.Outcome = "failed"
		fmt.Fprintln(os.Stderr, "start oracle generation:", err)
		return 1
	}
	_ = gateReader.Close()
	processID := process.Process.Pid
	observedExecutable, executableErr := sandbox.VerifyProcessExecutableEventually(context.Background(), processID, prepared.ExecutablePath, prepared.ExecutableSHA256, 2*time.Second)
	if executableErr != nil {
		_ = gateWriter.Close()
		_ = process.Process.Kill()
		_ = process.Wait()
		if accessCapture != nil {
			_, _ = accessCapture.Stop(processID)
		}
		execution.Outcome = "failed"
		fmt.Fprintln(os.Stderr, "verify oracle executable:", executableErr)
		return 1
	}
	execution.ExecutableObservedAt = time.Now().UTC()
	execution.ObservedExecutablePath = observedExecutable.Path
	execution.ObservedExecutableSHA256 = observedExecutable.SHA256
	execution.Access.ProcessID = processID
	var accessAttachErr error
	if accessCapture != nil {
		accessAttachErr = accessCapture.Attach(processID)
		if accessAttachErr != nil {
			execution.Access = evidence.AccessTelemetry{
				Status:    "unavailable",
				Source:    "macos-unified-log",
				ProcessID: processID,
				Reason:    accessAttachErr.Error(),
			}
		}
	}
	_ = gateWriter.Close()
	processErr := process.Wait()
	if accessCapture != nil {
		report, reportErr := accessCapture.Stop(processID)
		if reportErr != nil || accessAttachErr != nil {
			reason := accessAttachErr
			if reportErr != nil {
				reason = reportErr
			}
			execution.Access = evidence.AccessTelemetry{
				Status:    "unavailable",
				Source:    report.Source,
				ProcessID: processID,
				Reason:    reason.Error(),
			}
		} else {
			execution.Access = evidence.AccessTelemetry{
				Status:                       "captured",
				Source:                       report.Source,
				ProcessID:                    processID,
				ProcessIDs:                   append([]int(nil), report.ProcessIDs...),
				EventCount:                   len(report.Events),
				ParseErrors:                  report.ParseErrors,
				ProcessTreeErrors:            report.ProcessTreeErrors,
				ExecutableSampleCount:        report.ExecutableSampleCount,
				ExecutableSamplingIntervalMS: int(report.ExecutableSamplingInterval / time.Millisecond),
				ExecutableSamplingStartedAt:  report.ExecutableSamplingStartedAt,
				ExecutableSamplingStoppedAt:  report.ExecutableSamplingStoppedAt,
				ExecutableObservationCount:   len(report.ExecutableObservations),
				ExecutableObservationErrors:  report.ExecutableObservationErrors,
			}
			execution.AccessEvents = makeOracleAccessEvents(executionID, report.Events)
			execution.ExecutableObservations = makeOracleExecutableObservations(executionID, report.ExecutableObservations)
		}
	}
	if processErr != nil {
		execution.Outcome = "failed"
		fmt.Fprintln(os.Stderr, "oracle generation failed:", processErr)
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
	startGateFD := flags.Int("start-gate-fd", -1, "internal startup gate file descriptor")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *contractPath == "" || *policyHash == "" || *outputPath == "" {
		fmt.Fprintln(os.Stderr, "usage: sorna oracle generate --contract <path> --policy-sha256 <hash> --output <path>")
		return 2
	}
	if *startGateFD >= 0 {
		gate := os.NewFile(uintptr(*startGateFD), "oracle-start-gate")
		if gate == nil {
			fmt.Fprintln(os.Stderr, "oracle start gate file descriptor is invalid")
			return 1
		}
		_, gateErr := io.Copy(io.Discard, gate)
		_ = gate.Close()
		if gateErr != nil {
			fmt.Fprintln(os.Stderr, "wait for oracle start gate:", gateErr)
			return 1
		}
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

func makeOracleAccessEvents(executionID string, events []sandbox.AccessEvent) []evidence.OracleAccessEvent {
	converted := make([]evidence.OracleAccessEvent, 0, len(events))
	for index, event := range events {
		converted = append(converted, evidence.OracleAccessEvent{
			EventID:     fmt.Sprintf("access-%04d", index+1),
			ExecutionID: executionID,
			Sequence:    index + 1,
			Timestamp:   event.Timestamp,
			Process:     event.Process,
			PID:         event.PID,
			Decision:    event.Decision,
			Operation:   event.Operation,
			Resource:    event.Resource,
		})
	}
	return converted
}

func makeOracleExecutableObservations(executionID string, observations []sandbox.ExecutableObservation) []evidence.OracleExecutableObservation {
	converted := make([]evidence.OracleExecutableObservation, 0, len(observations))
	for index, observation := range observations {
		converted = append(converted, evidence.OracleExecutableObservation{
			EventID:     fmt.Sprintf("executable-%04d", index+1),
			ExecutionID: executionID,
			Sequence:    index + 1,
			Timestamp:   observation.Timestamp,
			PID:         observation.PID,
			Path:        observation.Path,
			SHA256:      observation.SHA256,
		})
	}
	return converted
}

func makeLifecycleAccessEvents(events []sandbox.AccessEvent) []lifecycle.AccessEvent {
	converted := make([]lifecycle.AccessEvent, 0, len(events))
	for _, event := range events {
		converted = append(converted, lifecycle.AccessEvent{
			Timestamp: event.Timestamp,
			Process:   event.Process,
			PID:       event.PID,
			Decision:  event.Decision,
			Operation: event.Operation,
			Resource:  event.Resource,
		})
	}
	return converted
}

func makeLifecycleExecutableObservations(observations []sandbox.ExecutableObservation) []lifecycle.ExecutableObservation {
	converted := make([]lifecycle.ExecutableObservation, 0, len(observations))
	for _, observation := range observations {
		converted = append(converted, lifecycle.ExecutableObservation{
			Timestamp: observation.Timestamp,
			PID:       observation.PID,
			Path:      observation.Path,
			SHA256:    observation.SHA256,
		})
	}
	return converted
}

func lifecycleObservationCoverage(access *lifecycle.AccessTelemetry) string {
	if access == nil || access.Status != "captured" {
		return "unavailable"
	}
	if access.ExecutableSampleCount == 0 || access.ExecutableObservationCount == 0 || access.ProcessTreeErrors > 0 || access.ExecutableObservationErrors > 0 {
		return "periodic-best-effort-with-gaps"
	}
	return "periodic-best-effort"
}

func lifecycleObservationLimitation(access *lifecycle.AccessTelemetry) string {
	if access.ExecutableSamplingIntervalMS > 0 {
		return fmt.Sprintf("executable identity was sampled every %d ms; transitions between samples may be unobserved", access.ExecutableSamplingIntervalMS)
	}
	return "executable identity coverage is periodic; transitions between samples may be unobserved"
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

func gateCommand(args []string) int {
	flags := flag.NewFlagSet("gate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	minimumCoverage := flags.String("minimum-observation-coverage", "", "minimum observation coverage required to pass: periodic-best-effort or periodic-best-effort-with-gaps")
	format := flags.String("format", "text", "output format: text, json, or ci-result")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if len(flags.Args()) != 1 || (*format != "text" && *format != "json" && *format != "ci-result") {
		fmt.Fprintln(os.Stderr, "usage: sorna gate [--minimum-observation-coverage <state>] [--format text|json|ci-result] <evidence-directory>")
		return 2
	}
	result, err := evidence.EvaluateGate(flags.Args()[0], evidence.GatePolicy{MinimumObservationCoverage: *minimumCoverage})
	if err != nil {
		if *format == "ci-result" {
			artifact, artifactErr := evidence.BuildCIErrorResult(flags.Args()[0], err)
			if artifactErr == nil {
				if writeErr := ciresult.WriteJSON(os.Stdout, artifact); writeErr == nil {
					return artifact.ExitCode
				}
			}
		}
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if *format == "ci-result" {
		artifact, err := evidence.BuildCIResult(flags.Args()[0], result)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		if err := ciresult.WriteJSON(os.Stdout, artifact); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	} else if *format == "json" {
		encoded, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		fmt.Println(string(encoded))
	} else {
		fmt.Printf("gate: %s\ncoverage: %s (%s)\n", result.Status, result.ObservationCoverage, result.ObservationPolicy)
		if result.ContractStatus != "" {
			fmt.Println("contract:", result.ContractStatus)
		}
		if result.MutationOutcome != "" {
			fmt.Println("mutation:", result.MutationOutcome)
		}
		if result.OracleOutcome != "" {
			fmt.Println("oracle:", result.OracleOutcome)
		}
		for _, warning := range result.Warnings {
			fmt.Println("warning:", warning)
		}
		for _, reason := range result.Reasons {
			fmt.Println("reason:", reason)
		}
	}
	return result.ExitCode
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

func mutationCommand(args []string) int {
	if len(args) < 1 {
		usage()
		return 2
	}
	switch args[0] {
	case "validate":
		return validateMutationCatalogue(args[1:])
	case "plan":
		return planMutationCampaign(args[1:])
	case "provider":
		return mutationProviderCommand(args[1:])
	case "run":
		return runMutationCampaign(args[1:])
	case "verify":
		return verifyMutationCampaign(args[1:])
	case "list":
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "usage: sorna mutation list <catalogue>")
			return 2
		}
		catalogue, err := mutation.LoadFile(args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		for _, spec := range catalogue.Mutations {
			fmt.Printf("%s\t%s\t%s\n", spec.ID, spec.Plane, spec.Description)
		}
		return 0
	default:
		fmt.Fprintln(os.Stderr, "unknown mutation command:", args[0])
		fmt.Fprintln(os.Stderr, "usage: sorna mutation validate <catalogue> [--contract <path>]")
		fmt.Fprintln(os.Stderr, "       sorna mutation plan <catalogue> --contract <path> --oracle <path> --baseline-evidence <dir> [--subject-policy <path>] [--output <path>]")
		fmt.Fprintln(os.Stderr, "       sorna mutation provider validate <path>")
		fmt.Fprintln(os.Stderr, "       sorna mutation provider inspect <plan> --provider <path> [--require-plan-binding] [--format text|json|ci-result] [--output <path>]")
		fmt.Fprintln(os.Stderr, "       sorna mutation provider preparation <summary> --provider <path> [--format ci-result] [--source-root <dir>] [--output <path>]")
		fmt.Fprintln(os.Stderr, "       sorna mutation run <plan> --provider <path> --oracle <path> --policy <path> [--subject-policy <path>] [--require-plan-binding] [--base-address <host:port>] [--output-dir <dir>] [--output <path>]")
		fmt.Fprintln(os.Stderr, "       sorna mutation verify <campaign-result> [--format text|ci-result] [--source-root <dir>] [--output <path>]")
		fmt.Fprintln(os.Stderr, "       sorna mutation list <catalogue>")
		return 2
	}
}

func validateMutationCatalogue(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: sorna mutation validate <catalogue> [--contract <path>]")
		return 2
	}
	flags := flag.NewFlagSet("mutation validate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	contractPath := flags.String("contract", "", "optional contract used to validate the catalogue binding")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if len(flags.Args()) != 0 {
		fmt.Fprintln(os.Stderr, "usage: sorna mutation validate <catalogue> [--contract <path>]")
		return 2
	}
	catalogue, err := mutation.LoadFile(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *contractPath != "" {
		document, err := contract.LoadFile(*contractPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		problems := mutation.ValidateAgainstContract(catalogue, document)
		if len(problems) > 0 {
			fmt.Fprintln(os.Stderr, "invalid mutation catalogue binding:")
			for _, problem := range problems {
				fmt.Fprintln(os.Stderr, "-", problem)
			}
			return 1
		}
	}
	if *contractPath == "" {
		fmt.Printf("valid: %s (%d mutations; structural only)\n", args[0], len(catalogue.Mutations))
	} else {
		fmt.Printf("valid: %s (%d mutations; contract %s)\n", args[0], len(catalogue.Mutations), catalogue.ContractID)
	}
	return 0
}

func mutationProviderCommand(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: sorna mutation provider validate <path>")
		fmt.Fprintln(os.Stderr, "       sorna mutation provider inspect <plan> --provider <path> [--require-plan-binding] [--format text|json|ci-result] [--output <path>]")
		fmt.Fprintln(os.Stderr, "       sorna mutation provider preparation <summary> --provider <path> [--format ci-result] [--source-root <dir>] [--output <path>]")
		return 2
	}
	switch args[0] {
	case "validate":
		return validateMutationProvider(args[1:])
	case "inspect":
		return inspectMutationProvider(args[1:])
	case "preparation":
		return mutationProviderPreparation(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "unknown mutation provider command:", args[0])
		fmt.Fprintln(os.Stderr, "usage: sorna mutation provider validate <path>")
		fmt.Fprintln(os.Stderr, "       sorna mutation provider inspect <plan> --provider <path> [--require-plan-binding] [--format text|json|ci-result] [--output <path>]")
		fmt.Fprintln(os.Stderr, "       sorna mutation provider preparation <summary> --provider <path> [--format ci-result] [--source-root <dir>] [--output <path>]")
		return 2
	}
}

func validateMutationProvider(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: sorna mutation provider validate <path>")
		return 2
	}
	provider, err := campaign.LoadProviderFile(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("valid: %s (%d entries; %d capabilities)\n", args[0], len(provider.Entries), len(provider.Capabilities))
	return 0
}

func inspectMutationProvider(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: sorna mutation provider inspect <plan> --provider <path> [--require-plan-binding] [--format text|json|ci-result] [--output <path>]")
		return 2
	}
	flags := flag.NewFlagSet("mutation provider inspect", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	providerPath := flags.String("provider", "", "provider manifest to review")
	requirePlanBinding := flags.Bool("require-plan-binding", false, "block providers that do not declare the reviewed plan hash")
	format := flags.String("format", "text", "review output format: text, json, or ci-result")
	outputPath := flags.String("output", "", "optional output path; existing files are not overwritten")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if len(flags.Args()) != 0 || *providerPath == "" || (*format != "text" && *format != "json" && *format != "ci-result") {
		fmt.Fprintln(os.Stderr, "usage: sorna mutation provider inspect <plan> --provider <path> [--require-plan-binding] [--format text|json|ci-result] [--output <path>]")
		return 2
	}
	planPath := args[0]
	planBytes, err := os.ReadFile(planPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read plan:", err)
		return 1
	}
	plan, err := campaign.LoadBytes(planPath, planBytes)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	providerBytes, err := os.ReadFile(*providerPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read provider:", err)
		return 1
	}
	provider, err := campaign.LoadProviderBytes(*providerPath, providerBytes)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	review := campaign.BuildProviderReview(campaign.ProviderReviewInput{
		PlanPath:           planPath,
		PlanSHA256:         campaign.HashBytes(planBytes),
		ProviderPath:       *providerPath,
		ProviderSHA256:     campaign.HashBytes(providerBytes),
		RequirePlanBinding: *requirePlanBinding,
		Plan:               plan,
		Provider:           provider,
	})
	var output []byte
	if *format == "ci-result" {
		artifact, artifactErr := evidence.BuildProviderReviewCIResult(review, ".")
		if artifactErr != nil {
			fmt.Fprintln(os.Stderr, "build provider review CI result:", artifactErr)
			return 1
		}
		var buffer bytes.Buffer
		if err := ciresult.WriteJSON(&buffer, artifact); err != nil {
			fmt.Fprintln(os.Stderr, "encode provider review CI result:", err)
			return 1
		}
		output = buffer.Bytes()
	} else if *format == "json" {
		output, err = json.MarshalIndent(review, "", "  ")
		if err == nil {
			output = append(output, '\n')
		}
	} else {
		output = []byte(formatProviderReview(review))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "encode provider review:", err)
		return 1
	}
	if *outputPath != "" {
		if err := os.MkdirAll(filepath.Dir(*outputPath), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "create provider review output directory:", err)
			return 1
		}
		file, err := os.OpenFile(*outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if os.IsExist(err) {
				fmt.Fprintln(os.Stderr, "provider review output already exists:", *outputPath)
			} else {
				fmt.Fprintln(os.Stderr, "open provider review output:", err)
			}
			return 1
		}
		if _, err := file.Write(output); err != nil {
			_ = file.Close()
			fmt.Fprintln(os.Stderr, "write provider review:", err)
			return 1
		}
		if err := file.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "close provider review:", err)
			return 1
		}
		fmt.Printf("review: %s\nstatus: %s\n", *outputPath, review.Status)
	} else {
		_, _ = os.Stdout.Write(output)
	}
	if review.Status != "ready" {
		return 1
	}
	return 0
}

func mutationProviderPreparation(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: sorna mutation provider preparation <summary> --provider <path> [--format ci-result] [--source-root <dir>] [--output <path>]")
		return 2
	}
	flags := flag.NewFlagSet("mutation provider preparation", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	providerPath := flags.String("provider", "", "provider manifest bound to the preparation summary")
	format := flags.String("format", "ci-result", "output format: ci-result")
	sourceRoot := flags.String("source-root", ".", "source root recorded in the CI result")
	outputPath := flags.String("output", "", "optional CI result output path; existing files are not overwritten")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if len(flags.Args()) != 0 || *providerPath == "" || *format != "ci-result" {
		fmt.Fprintln(os.Stderr, "usage: sorna mutation provider preparation <summary> --provider <path> [--format ci-result] [--source-root <dir>] [--output <path>]")
		return 2
	}
	summaryPath := args[0]
	artifact, err := evidence.BuildMutationPreparationCIResult(summaryPath, *providerPath, *sourceRoot)
	if err != nil {
		errorArtifact, artifactErr := evidence.BuildMutationPreparationCIErrorResult(summaryPath, *providerPath, *sourceRoot, err)
		if artifactErr != nil {
			fmt.Fprintln(os.Stderr, "build mutation preparation CI error result:", artifactErr)
			return 2
		}
		return emitCIResult(errorArtifact, *outputPath, "mutation preparation")
	}
	return emitCIResult(artifact, *outputPath, "mutation preparation")
}

func formatProviderReview(review campaign.ProviderReview) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "provider: %s (%s@%d)\n", review.Provider.Path, review.Provider.ID, review.Provider.Version)
	fmt.Fprintf(&builder, "plan: %s\n", review.Plan.Path)
	fmt.Fprintf(&builder, "plan hash: %s\n", review.Plan.SHA256)
	fmt.Fprintf(&builder, "provider hash: %s\n", review.Provider.SHA256)
	fmt.Fprintf(&builder, "plan binding: %s\n", review.Provider.PlanBinding)
	fmt.Fprintf(&builder, "status: %s\n", review.Status)
	builder.WriteString("capabilities:\n")
	for _, capability := range review.Capabilities {
		fmt.Fprintf(&builder, "  - %s %s %s\n", capability.Plane, capability.Operator, capability.Target)
	}
	builder.WriteString("mutations:\n")
	for _, mutation := range review.Mutations {
		fmt.Fprintf(&builder, "  %d %s: %s (entry=%s, capability=%s)\n", mutation.Sequence, mutation.MutationID, mutation.Status, mutation.EntryStatus, mutation.CapabilityStatus)
	}
	return builder.String()
}

func runMutationCampaign(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: sorna mutation run <plan> --provider <path> --oracle <path> --policy <path> [--subject-policy <path>] [--require-plan-binding] [--base-address <host:port>] [--output-dir <dir>] [--output <path>]")
		return 2
	}
	flags := flag.NewFlagSet("mutation run", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	providerPath := flags.String("provider", "", "provider manifest for prepared subject variants")
	oraclePath := flags.String("oracle", "", "path to the frozen oracle artifact")
	policyPath := flags.String("policy", "", "path to the oracle isolation policy")
	subjectPolicyPath := flags.String("subject-policy", "", "path to the managed-subject isolation policy")
	requirePlanBinding := flags.Bool("require-plan-binding", false, "reject providers that do not declare the reviewed plan hash")
	baseAddress := flags.String("base-address", "127.0.0.1:8081", "first subject address; each mutation receives the next port")
	readyPath := flags.String("ready-path", "/healthz", "HTTP path used to wait for each subject")
	outputDir := flags.String("output-dir", ".artifacts/document-pipeline-campaign", "root directory for per-mutation evidence")
	outputPath := flags.String("output", "", "campaign result output path; defaults inside output-dir")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if len(flags.Args()) != 0 || *providerPath == "" || *oraclePath == "" || *policyPath == "" {
		fmt.Fprintln(os.Stderr, "usage: sorna mutation run <plan> --provider <path> --oracle <path> --policy <path> [--subject-policy <path>] [--require-plan-binding] [--base-address <host:port>] [--output-dir <dir>] [--output <path>]")
		return 2
	}
	planPath := args[0]
	planBytes, err := os.ReadFile(planPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	plan, err := campaign.LoadBytes(planPath, planBytes)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	planHash := campaign.HashBytes(planBytes)
	planSemanticHash, err := campaign.SemanticHash(plan)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	providerBytes, err := os.ReadFile(*providerPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	provider, err := campaign.LoadProviderBytes(*providerPath, providerBytes)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if problems := provider.ValidateForPlan(plan); len(problems) > 0 {
		fmt.Fprintln(os.Stderr, "provider does not cover campaign plan:")
		for _, problem := range problems {
			fmt.Fprintln(os.Stderr, "-", problem)
		}
		return 1
	}
	if *requirePlanBinding && provider.PlanSHA256 == "" {
		fmt.Fprintln(os.Stderr, "campaign provider does not declare the reviewed plan hash")
		return 1
	}
	if provider.PlanSHA256 != "" && provider.PlanSHA256 != planHash {
		fmt.Fprintf(os.Stderr, "campaign provider was built from plan hash %q, want %q\n", provider.PlanSHA256, planHash)
		return 1
	}
	oracleArtifact, err := oracle.LoadFile(*oraclePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	oracleHash, err := oracle.Hash(oracleArtifact)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hash campaign oracle:", err)
		return 1
	}
	if oracleHash != plan.Oracle.SHA256 || oracleArtifact.Schema != plan.Oracle.Schema || oracleArtifact.Contract.ID != plan.Contract.ID || oracleArtifact.Contract.Version != plan.Contract.Version || oracleArtifact.Contract.SHA256 != plan.Contract.SHA256 {
		fmt.Fprintln(os.Stderr, "campaign oracle does not match the plan")
		return 1
	}
	oraclePolicy, err := policy.LoadFile(*policyPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	sealedOraclePolicy, err := policy.Seal(oraclePolicy)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if sealedOraclePolicy.SHA256 != plan.OraclePolicySHA256 {
		fmt.Fprintln(os.Stderr, "campaign oracle policy does not match the plan")
		return 1
	}
	subjectPolicyHash := ""
	if *subjectPolicyPath != "" {
		document, err := policy.LoadFile(*subjectPolicyPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		sealed, err := policy.Seal(document)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		subjectPolicyHash = sealed.SHA256
	}
	if subjectPolicyHash != plan.SubjectPolicySHA256 {
		fmt.Fprintf(os.Stderr, "campaign subject policy hash %q does not match plan hash %q\n", subjectPolicyHash, plan.SubjectPolicySHA256)
		return 1
	}
	if pathsOverlap(*outputDir, plan.Baseline.EvidencePath) {
		fmt.Fprintln(os.Stderr, "campaign output directory must be separate from baseline evidence")
		return 2
	}
	host, portText, err := net.SplitHostPort(*baseAddress)
	if err != nil || strings.TrimSpace(host) == "" {
		fmt.Fprintln(os.Stderr, "base address must have the form host:port")
		return 2
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 || port+len(plan.Mutations)-1 > 65535 {
		fmt.Fprintln(os.Stderr, "base address port must leave room for every mutation")
		return 2
	}
	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "create campaign output directory:", err)
		return 1
	}
	resultPath := *outputPath
	if resultPath == "" {
		resultPath = filepath.Join(*outputDir, "campaign-result.json")
	}
	if pathsOverlap(resultPath, plan.Baseline.EvidencePath) {
		fmt.Fprintln(os.Stderr, "campaign result output must be separate from baseline evidence")
		return 2
	}
	startedAt := time.Now().UTC()
	entries := make([]campaign.EntryResult, 0, len(plan.Mutations))
	for _, planned := range plan.Mutations {
		address := net.JoinHostPort(host, strconv.Itoa(port+planned.Sequence-1))
		baseURL := "http://" + address
		prepared, err := provider.Resolve(planned.Spec.ID, address, baseURL)
		if err != nil {
			entries = append(entries, campaign.EntryResult{Sequence: planned.Sequence, MutationID: planned.Spec.ID, EvidencePath: campaignEntryPath(*outputDir, planned), Status: "error", ExitCode: -1, Reason: err.Error()})
			continue
		}
		if len(prepared.Command) == 0 {
			entries = append(entries, campaign.EntryResult{Sequence: planned.Sequence, MutationID: planned.Spec.ID, EvidencePath: campaignEntryPath(*outputDir, planned), Status: "error", ExitCode: -1, Reason: "provider resolved an empty subject command"})
			continue
		}
		if err := campaign.VerifyPreparedSubject(prepared); err != nil {
			entries = append(entries, campaign.EntryResult{Sequence: planned.Sequence, MutationID: planned.Spec.ID, EvidencePath: campaignEntryPath(*outputDir, planned), Status: "error", ExitCode: -1, Reason: fmt.Sprintf("verify prepared subject: %v", err)})
			continue
		}
		entryDir := campaignEntryPath(*outputDir, planned)
		if _, statErr := os.Stat(entryDir); statErr == nil {
			entries = append(entries, campaign.EntryResult{Sequence: planned.Sequence, MutationID: planned.Spec.ID, EvidencePath: entryDir, Status: "error", ExitCode: -1, Reason: "mutation evidence directory already exists"})
			continue
		} else if !os.IsNotExist(statErr) {
			entries = append(entries, campaign.EntryResult{Sequence: planned.Sequence, MutationID: planned.Spec.ID, EvidencePath: entryDir, Status: "error", ExitCode: -1, Reason: statErr.Error()})
			continue
		}
		runArgs := []string{
			"run",
			"--oracle", *oraclePath,
			"--policy", *policyPath,
			"--subject-root", prepared.SubjectRoot,
			"--base-url", baseURL,
			"--subject-command", prepared.Command[0],
			"--baseline-evidence", plan.Baseline.EvidencePath,
			"--ready-path", *readyPath,
			"--subject-variant", prepared.Variant,
			"--mutation-id", planned.Spec.ID,
			"--mutation-plane", planned.Spec.Plane,
			"--mutation-description", planned.Spec.Description,
			"--output-dir", entryDir,
		}
		if *subjectPolicyPath != "" {
			runArgs = append(runArgs, "--subject-policy", *subjectPolicyPath)
		}
		if prepared.SubjectDir != "" {
			runArgs = append(runArgs, "--subject-dir", prepared.SubjectDir)
		}
		for _, argument := range prepared.Command[1:] {
			runArgs = append(runArgs, "--subject-arg", argument)
		}
		for _, ruleID := range planned.Spec.ExpectedRuleIDs {
			runArgs = append(runArgs, "--expected-rule", ruleID)
		}
		fmt.Printf("campaign mutation %d/%d: %s\n", planned.Sequence, len(plan.Mutations), planned.Spec.ID)
		process := osexec.Command(os.Args[0], runArgs...)
		process.Stdin = os.Stdin
		process.Stdout = os.Stdout
		process.Stderr = os.Stderr
		runErr := process.Run()
		exitCode := 0
		if runErr != nil {
			exitCode = -1
			var exitErr *osexec.ExitError
			if errors.As(runErr, &exitErr) {
				exitCode = exitErr.ExitCode()
			}
		}
		entryResult := campaign.EntryResult{Sequence: planned.Sequence, MutationID: planned.Spec.ID, EvidencePath: entryDir, ExitCode: exitCode}
		if provenanceErr := evidence.AttachCampaignProvenance(entryDir, evidence.CampaignProvenanceInput{
			PlanPath:      planPath,
			PlanBytes:     planBytes,
			ProviderPath:  *providerPath,
			ProviderBytes: providerBytes,
			Sequence:      planned.Sequence,
			MutationID:    planned.Spec.ID,
		}); provenanceErr != nil {
			entryResult.Status = "error"
			entryResult.Reason = fmt.Sprintf("attach campaign provenance: %v", provenanceErr)
		} else if verifyErr := evidence.Verify(entryDir); verifyErr != nil {
			entryResult.Status = "error"
			entryResult.Reason = fmt.Sprintf("verify mutation evidence: %v", verifyErr)
		} else if evidenceHashes, hashErr := campaign.HashEvidence(entryDir); hashErr != nil {
			entryResult.Status = "error"
			entryResult.Reason = fmt.Sprintf("hash mutation evidence: %v", hashErr)
		} else {
			entryResult.Evidence = &evidenceHashes
			record, readErr := readRunRecord(entryDir)
			if readErr != nil {
				entryResult.Status = "error"
				entryResult.Reason = readErr.Error()
			} else if record.Mutation == nil {
				entryResult.Status = "error"
				entryResult.Reason = "mutation evidence does not contain a mutation result"
			} else {
				entryResult.RunID = record.RunID
				entryResult.Outcome = record.Mutation.Outcome
				entryResult.Reason = record.Mutation.Reason
				entryResult.Diagnosis = campaignDiagnosis(*record.Mutation)
				if record.Mutation.Outcome == "killed" {
					entryResult.Status = "passed"
				} else {
					entryResult.Status = "failed"
				}
			}
		}
		if runErr != nil && entryResult.Status == "" {
			entryResult.Status = "error"
			entryResult.Reason = runErr.Error()
		}
		entries = append(entries, entryResult)
	}
	result := buildCampaignResult(planPath, planHash, planSemanticHash, startedAt, entries)
	resultHash, err := campaign.WriteResult(resultPath, result)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Printf("campaign result: %s\nstatus: %s\nplan: %s\nresult hash: %s\n", resultPath, result.Status, planHash, resultHash)
	return campaignExitCode(result.Status)
}

func verifyMutationCampaign(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: sorna mutation verify <campaign-result> [--format text|ci-result] [--source-root <dir>] [--output <path>]")
		return 2
	}
	flags := flag.NewFlagSet("mutation verify", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	format := flags.String("format", "text", "output format: text or ci-result")
	sourceRoot := flags.String("source-root", ".", "source root recorded in the CI result")
	outputPath := flags.String("output", "", "optional CI result output path; existing files are not overwritten")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if len(flags.Args()) != 0 || (*format != "text" && *format != "ci-result") {
		fmt.Fprintln(os.Stderr, "usage: sorna mutation verify <campaign-result> [--format text|ci-result] [--source-root <dir>] [--output <path>]")
		return 2
	}
	campaignPath := args[0]
	result, err := campaign.LoadResult(campaignPath)
	if err != nil {
		if *format == "ci-result" {
			artifact, artifactErr := evidence.BuildMutationCampaignCIErrorResult(campaignPath, *sourceRoot, err)
			if artifactErr == nil {
				return emitMutationCampaignCIResult(artifact, *outputPath)
			}
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := verifyCampaignPlanReference(result.Plan, *sourceRoot); err != nil {
		if *format == "ci-result" {
			artifact, artifactErr := evidence.BuildMutationCampaignCIErrorResult(campaignPath, *sourceRoot, err)
			if artifactErr == nil {
				return emitMutationCampaignCIResult(artifact, *outputPath)
			}
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	verified := 0
	for _, entry := range result.Entries {
		if entry.Evidence == nil {
			continue
		}
		if err := evidence.Verify(entry.EvidencePath); err != nil {
			if *format == "ci-result" {
				artifact, artifactErr := evidence.BuildMutationCampaignCIErrorResult(campaignPath, *sourceRoot, fmt.Errorf("verify mutation %s evidence: %w", entry.MutationID, err))
				if artifactErr == nil {
					return emitMutationCampaignCIResult(artifact, *outputPath)
				}
			}
			fmt.Fprintf(os.Stderr, "verify mutation %s evidence: %v\n", entry.MutationID, err)
			return 1
		}
		actual, err := campaign.HashEvidence(entry.EvidencePath)
		if err != nil {
			if *format == "ci-result" {
				artifact, artifactErr := evidence.BuildMutationCampaignCIErrorResult(campaignPath, *sourceRoot, fmt.Errorf("hash mutation %s evidence: %w", entry.MutationID, err))
				if artifactErr == nil {
					return emitMutationCampaignCIResult(artifact, *outputPath)
				}
			}
			fmt.Fprintf(os.Stderr, "hash mutation %s evidence: %v\n", entry.MutationID, err)
			return 1
		}
		if actual != *entry.Evidence {
			if *format == "ci-result" {
				artifact, artifactErr := evidence.BuildMutationCampaignCIErrorResult(campaignPath, *sourceRoot, fmt.Errorf("mutation %s evidence hashes do not match campaign result", entry.MutationID))
				if artifactErr == nil {
					return emitMutationCampaignCIResult(artifact, *outputPath)
				}
			}
			fmt.Fprintf(os.Stderr, "mutation %s evidence hashes do not match campaign result\n", entry.MutationID)
			return 1
		}
		verified++
	}
	if *format == "ci-result" {
		artifact, err := evidence.BuildMutationCampaignCIResult(result, campaignPath, *sourceRoot)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		return emitMutationCampaignCIResult(artifact, *outputPath)
	}
	fmt.Printf("verified: %s (%d evidence entries)\n", campaignPath, verified)
	return 0
}

func verifyCampaignPlanReference(reference campaign.PlanReference, sourceRoot string) error {
	planPath := reference.Path
	if !filepath.IsAbs(planPath) && sourceRoot != "." {
		planPath = filepath.Join(sourceRoot, planPath)
	}
	planBytes, err := os.ReadFile(planPath)
	if err != nil {
		return fmt.Errorf("read campaign plan %s: %w", planPath, err)
	}
	if actual := campaign.HashBytes(planBytes); actual != reference.SHA256 {
		return fmt.Errorf("campaign plan %s exact hash %q does not match result %q", planPath, actual, reference.SHA256)
	}
	plan, err := campaign.LoadBytes(planPath, planBytes)
	if err != nil {
		return fmt.Errorf("validate campaign plan %s: %w", planPath, err)
	}
	if reference.SemanticSHA256 != "" {
		semanticHash, err := campaign.SemanticHash(plan)
		if err != nil {
			return fmt.Errorf("hash campaign semantic plan identity: %w", err)
		}
		if semanticHash != reference.SemanticSHA256 {
			return fmt.Errorf("campaign plan %s semantic hash %q does not match result %q", planPath, semanticHash, reference.SemanticSHA256)
		}
	}
	return nil
}

func emitMutationCampaignCIResult(artifact ciresult.Artifact, outputPath string) int {
	return emitCIResult(artifact, outputPath, "mutation campaign")
}

func emitCIResult(artifact ciresult.Artifact, outputPath, label string) int {
	var buffer bytes.Buffer
	if err := ciresult.WriteJSON(&buffer, artifact); err != nil {
		fmt.Fprintf(os.Stderr, "encode %s CI result: %v\n", label, err)
		return 2
	}
	if outputPath == "" {
		if _, err := os.Stdout.Write(buffer.Bytes()); err != nil {
			fmt.Fprintf(os.Stderr, "write %s CI result: %v\n", label, err)
			return 2
		}
		return artifact.ExitCode
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create %s CI result directory: %v\n", label, err)
		return 2
	}
	file, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			fmt.Fprintf(os.Stderr, "%s CI result output already exists: %s\n", label, outputPath)
		} else {
			fmt.Fprintf(os.Stderr, "open %s CI result output: %v\n", label, err)
		}
		return 2
	}
	if _, err := file.Write(buffer.Bytes()); err != nil {
		_ = file.Close()
		fmt.Fprintf(os.Stderr, "write %s CI result: %v\n", label, err)
		return 2
	}
	if err := file.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "close %s CI result: %v\n", label, err)
		return 2
	}
	fmt.Printf("CI result: %s\nstatus: %s\n", outputPath, artifact.Status)
	return artifact.ExitCode
}

func campaignEntryPath(outputDir string, planned campaign.MutationEntry) string {
	return filepath.Join(outputDir, fmt.Sprintf("%03d-%s", planned.Sequence, safeCampaignID(planned.Spec.ID)))
}

func safeCampaignID(id string) string {
	var builder strings.Builder
	for _, character := range id {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			builder.WriteRune(character)
		} else {
			builder.WriteByte('-')
		}
	}
	value := strings.Trim(builder.String(), "-.")
	if value == "" {
		return "mutation"
	}
	return value
}

func pathsOverlap(left, right string) bool {
	leftAbsolute, leftErr := filepath.Abs(left)
	rightAbsolute, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return true
	}
	leftAbsolute = filepath.Clean(leftAbsolute)
	rightAbsolute = filepath.Clean(rightAbsolute)
	return leftAbsolute == rightAbsolute || pathContains(leftAbsolute, rightAbsolute) || pathContains(rightAbsolute, leftAbsolute)
}

func pathContains(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func readRunRecord(outputDir string) (runner.RunRecord, error) {
	contents, err := os.ReadFile(filepath.Join(outputDir, "run.json"))
	if err != nil {
		return runner.RunRecord{}, fmt.Errorf("read mutation run record: %w", err)
	}
	var record runner.RunRecord
	if err := json.Unmarshal(contents, &record); err != nil {
		return runner.RunRecord{}, fmt.Errorf("decode mutation run record: %w", err)
	}
	return record, nil
}

func campaignDiagnosis(result mutation.Result) *campaign.Diagnosis {
	expected := make(map[string]string, len(result.ExpectedObservations))
	for _, observation := range result.ExpectedObservations {
		expected[observation.RuleID] = observation.Status
	}
	return &campaign.Diagnosis{
		ExpectedRuleStatus:    expected,
		DirectlyFailedRules:   append([]string(nil), result.DirectlyFailedRules...),
		CascadingInconclusive: append([]string(nil), result.CascadingInconclusive...),
		UnaffectedRules:       append([]string(nil), result.UnaffectedRules...),
		UnobservedExpected:    append([]string(nil), result.UnobservedExpected...),
	}
}

func buildCampaignResult(planPath, planHash, planSemanticHash string, startedAt time.Time, entries []campaign.EntryResult) campaign.Result {
	result := campaign.Result{
		Schema:     campaign.ResultSchema,
		Status:     "passed",
		Plan:       campaign.PlanReference{Path: planPath, SHA256: planHash, SemanticSHA256: planSemanticHash},
		StartedAt:  startedAt,
		FinishedAt: time.Now().UTC(),
		Summary:    campaign.Summary{Total: len(entries)},
		Entries:    entries,
	}
	for _, entry := range entries {
		switch entry.Status {
		case "error":
			result.Summary.Errors++
			result.Status = "error"
		case "failed":
			if result.Status == "passed" {
				result.Status = "failed"
			}
		}
		switch entry.Outcome {
		case "killed":
			result.Summary.Killed++
		case "survived":
			result.Summary.Survived++
		case "inconclusive":
			result.Summary.Inconclusive++
		case "":
		default:
			result.Summary.Other++
		}
	}
	return result
}

func campaignExitCode(status string) int {
	switch status {
	case "passed":
		return 0
	case "failed":
		return 1
	default:
		return 2
	}
}

func planMutationCampaign(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: sorna mutation plan <catalogue> --contract <path> --oracle <path> --baseline-evidence <dir> [--subject-policy <path>] [--output <path>]")
		return 2
	}
	flags := flag.NewFlagSet("mutation plan", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	contractPath := flags.String("contract", "", "path to the contract used to validate the catalogue binding")
	oraclePath := flags.String("oracle", "", "path to a canonical frozen oracle artifact")
	baselinePath := flags.String("baseline-evidence", "", "passing unmutated evidence bundle used for comparison")
	subjectPolicyPath := flags.String("subject-policy", "", "optional managed-subject policy used by the baseline")
	outputPath := flags.String("output", ".artifacts/document-pipeline-mutation-plan.json", "output path for the canonical campaign plan")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if len(flags.Args()) != 0 || *contractPath == "" || *oraclePath == "" || *baselinePath == "" {
		fmt.Fprintln(os.Stderr, "usage: sorna mutation plan <catalogue> --contract <path> --oracle <path> --baseline-evidence <dir> [--subject-policy <path>] [--output <path>]")
		return 2
	}
	catalogue, err := mutation.LoadFile(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	catalogueHash, err := campaign.HashFile(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "hash mutation catalogue:", err)
		return 1
	}
	contractDocument, err := contract.LoadFile(*contractPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	contractPathAbsolute, err := filepath.Abs(*contractPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolve campaign contract path:", err)
		return 1
	}
	sealedContract, err := contract.SealAt(contractDocument, filepath.Dir(contractPathAbsolute))
	if err != nil {
		fmt.Fprintln(os.Stderr, "seal campaign contract:", err)
		return 1
	}
	oracleArtifact, err := oracle.LoadFile(*oraclePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	oracleHash, err := oracle.Hash(oracleArtifact)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hash frozen oracle:", err)
		return 1
	}
	if sealedContract.SHA256 != oracleArtifact.Contract.SHA256 {
		fmt.Fprintf(os.Stderr, "campaign contract hash %q does not match frozen oracle contract hash %q\n", sealedContract.SHA256, oracleArtifact.Contract.SHA256)
		return 1
	}
	contractReference := runner.ContractReference{
		ID:      oracleArtifact.Contract.ID,
		Version: oracleArtifact.Contract.Version,
		SHA256:  oracleArtifact.Contract.SHA256,
	}
	oracleReference := &runner.OracleReference{Schema: oracleArtifact.Schema, SHA256: oracleHash}
	subjectPolicyHash := ""
	if *subjectPolicyPath != "" {
		document, err := policy.LoadFile(*subjectPolicyPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		sealed, err := policy.Seal(document)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		subjectPolicyHash = sealed.SHA256
	}
	baseline, err := evidence.ValidateBaseline(*baselinePath, evidence.BaselineRequirements{
		Contract:            contractReference,
		Oracle:              oracleReference,
		PolicySHA256:        oracleArtifact.PolicySHA256,
		SubjectPolicySHA256: subjectPolicyHash,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	plan, err := campaign.Build(campaign.BuildRequest{
		CataloguePath:       args[0],
		CatalogueSHA256:     catalogueHash,
		Catalogue:           catalogue,
		Contract:            contractDocument,
		ContractSHA256:      sealedContract.SHA256,
		Oracle:              oracleArtifact,
		Baseline:            baseline,
		SubjectPolicySHA256: subjectPolicyHash,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	planHash, err := campaign.WriteFile(*outputPath, plan)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	semanticHash, err := campaign.SemanticHash(plan)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("plan:", *outputPath)
	fmt.Println("hash:", planHash)
	fmt.Println("semantic hash:", semanticHash)
	fmt.Println("mutations:", len(plan.Mutations))
	return 0
}

func runSubject(args []string) int {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	contractPath := flags.String("contract", "", "path to the contract")
	oraclePath := flags.String("oracle", "", "path to a canonical frozen oracle artifact")
	policyPath := flags.String("policy", "", "path to an isolation policy")
	subjectPolicyPath := flags.String("subject-policy", "", "path to a managed-subject isolation policy")
	subjectRoot := flags.String("subject-root", ".", "root used to resolve managed-subject policy paths")
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
	mutationPlane := flags.String("mutation-plane", "implementation", "mutation plane")
	mutationDescription := flags.String("mutation-description", "", "description of the mutation")
	var expectedRules stringList
	flags.Var(&expectedRules, "expected-rule", "rule ID expected to observe the mutation; may be repeated")
	baselineEvidencePath := flags.String("baseline-evidence", "", "passing unmutated evidence bundle required for mutation comparison")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if (*contractPath == "" && *oraclePath == "") || *baseURL == "" {
		fmt.Fprintln(os.Stderr, "usage: sorna run [--oracle <path> | --contract <path>] [--policy <path>] --base-url <url> [--subject-command <executable> --subject-arg <arg> ...] [--subject-policy <path>] [--baseline-evidence <dir>] [--output-dir <dir>]")
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
	if *subjectPolicyPath != "" && *subjectCommand == "" {
		fmt.Fprintln(os.Stderr, "--subject-policy requires --subject-command")
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
	var sealedSubjectPolicy *policy.Sealed
	if *subjectPolicyPath != "" {
		policyDocument, err := policy.LoadFile(*subjectPolicyPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		sealedValue, err := policy.Seal(policyDocument)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		sealedSubjectPolicy = &sealedValue
	}
	expectedSubjectID := oracleArtifact.Contract.ID
	if !usingOracle {
		expectedSubjectID, _ = sealed.Document.Contract["id"].(string)
	}
	for name, candidate := range map[string]*policy.Sealed{
		"oracle":  sealedPolicy,
		"subject": sealedSubjectPolicy,
	} {
		if candidate == nil {
			continue
		}
		candidateSubjectID, subjectErr := policy.SubjectID(candidate.Document)
		if subjectErr != nil {
			fmt.Fprintln(os.Stderr, subjectErr)
			return 1
		}
		if candidateSubjectID != expectedSubjectID {
			fmt.Fprintf(os.Stderr, "%s policy subject ID %q does not match contract ID %q\n", name, candidateSubjectID, expectedSubjectID)
			return 1
		}
	}
	if usingOracle && sealedPolicy != nil && oracleArtifact.PolicySHA256 != sealedPolicy.SHA256 {
		fmt.Fprintln(os.Stderr, "oracle policy hash does not match the supplied policy")
		return 1
	}
	expectedContract, expectedOracle, err := runReferences(usingOracle, sealed, oracleArtifact)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var mutationSpec *mutation.Spec
	var baselineReference *runner.BaselineReference
	if *mutationID != "" {
		if len(expectedRules) == 0 {
			fmt.Fprintln(os.Stderr, "--expected-rule is required when --mutation-id is provided")
			return 2
		}
		if *baselineEvidencePath == "" {
			fmt.Fprintln(os.Stderr, "--baseline-evidence is required when --mutation-id is provided")
			return 2
		}
		if sameDirectory(*baselineEvidencePath, *outputDir) {
			fmt.Fprintln(os.Stderr, "--baseline-evidence must be different from --output-dir")
			return 2
		}
		mutationSpec = &mutation.Spec{
			ID:              *mutationID,
			Plane:           *mutationPlane,
			Description:     *mutationDescription,
			ExpectedRuleIDs: append([]string(nil), expectedRules...),
		}
		baseline, baselineErr := evidence.ValidateBaseline(*baselineEvidencePath, evidence.BaselineRequirements{
			Contract:            expectedContract,
			Oracle:              expectedOracle,
			PolicySHA256:        sealedPolicyHash(sealedPolicy),
			SubjectPolicySHA256: sealedPolicyHash(sealedSubjectPolicy),
		})
		if baselineErr != nil {
			fmt.Fprintln(os.Stderr, baselineErr)
			return 1
		}
		baselineReference = &baseline
	}

	var managedSubject *lifecycle.Process
	var subjectSandbox *lifecycle.SandboxRecord
	var subjectAccessCapture sandbox.AccessCapture
	var subjectAccessAttachErr error
	lifecycleRecord := lifecycle.External(*baseURL, nil)
	if *subjectCommand != "" {
		command := append([]string{*subjectCommand}, subjectArgs...)
		launchCommand := append([]string(nil), command...)
		var subjectAccess *lifecycle.AccessTelemetry
		if sealedSubjectPolicy != nil {
			prepared, prepareErr := sandbox.Prepare(command, *subjectRoot, *sealedSubjectPolicy)
			if prepareErr != nil {
				fmt.Fprintln(os.Stderr, prepareErr)
				return 1
			}
			launchCommand = prepared.Command
			subjectSandbox = &lifecycle.SandboxRecord{
				Backend:          prepared.Backend,
				Enforcement:      prepared.Enforcement,
				PolicySHA256:     prepared.PolicySHA256,
				SubjectID:        prepared.SubjectID,
				ExecutablePath:   prepared.ExecutablePath,
				ExecutableSHA256: prepared.ExecutableSHA256,
				CanInvokeSubject: prepared.CanInvokeSubject,
				AllowedTools:     append([]string(nil), prepared.AllowedTools...),
			}
			subjectAccessCapture, err = sandbox.StartAccessCapture(context.Background())
			if err != nil {
				subjectAccess = &lifecycle.AccessTelemetry{
					Status: "unavailable",
					Source: "macos-unified-log",
					Reason: err.Error(),
				}
			}
		}
		managedSubject, err = lifecycle.Start(context.Background(), lifecycle.Config{
			Command:         launchCommand,
			RecordCommand:   command,
			Dir:             *subjectDir,
			BaseURL:         *baseURL,
			ReadyPath:       *readyPath,
			StartupTimeout:  *startupTimeout,
			ShutdownTimeout: *shutdownTimeout,
			Sandbox:         subjectSandbox,
			Access:          subjectAccess,
		})
		if err != nil {
			if subjectAccessCapture != nil {
				_, _ = subjectAccessCapture.Stop(0)
			}
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if subjectAccessCapture != nil {
			subjectAccessAttachErr = subjectAccessCapture.Attach(managedSubject.PID())
			if subjectAccessAttachErr != nil {
				subjectAccess = &lifecycle.AccessTelemetry{
					Status:    "unavailable",
					Source:    "macos-unified-log",
					ProcessID: managedSubject.PID(),
					Reason:    subjectAccessAttachErr.Error(),
				}
			}
		}
		if subjectSandbox != nil {
			observedExecutable, executableErr := sandbox.VerifyProcessExecutableEventually(context.Background(), managedSubject.PID(), subjectSandbox.ExecutablePath, subjectSandbox.ExecutableSHA256, 2*time.Second)
			if executableErr != nil {
				_ = managedSubject.Close(context.Background())
				if subjectAccessCapture != nil {
					_, _ = subjectAccessCapture.Stop(0)
				}
				fmt.Fprintln(os.Stderr, "verify subject executable:", executableErr)
				return 1
			}
			subjectSandbox.ObservedExecutablePath = observedExecutable.Path
			subjectSandbox.ObservedExecutableSHA256 = observedExecutable.SHA256
			subjectSandbox.ExecutableObservedAt = time.Now().UTC()
		}
		lifecycleRecord = managedSubject.Record()
		lifecycleRecord.Sandbox = subjectSandbox
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
		if subjectSandbox != nil {
			lifecycleRecord.Sandbox = subjectSandbox
		}
		if subjectAccessCapture != nil {
			report, reportErr := subjectAccessCapture.Stop(managedSubject.PID())
			if reportErr != nil || subjectAccessAttachErr != nil {
				reason := subjectAccessAttachErr
				if reportErr != nil {
					reason = reportErr
				}
				lifecycleRecord.Access = &lifecycle.AccessTelemetry{
					Status:    "unavailable",
					Source:    report.Source,
					ProcessID: managedSubject.PID(),
					Reason:    reason.Error(),
				}
			} else {
				lifecycleRecord.Access = &lifecycle.AccessTelemetry{
					Status:                       "captured",
					Source:                       report.Source,
					ProcessID:                    managedSubject.PID(),
					ProcessIDs:                   append([]int(nil), report.ProcessIDs...),
					EventCount:                   len(report.Events),
					ParseErrors:                  report.ParseErrors,
					ProcessTreeErrors:            report.ProcessTreeErrors,
					ExecutableSampleCount:        report.ExecutableSampleCount,
					ExecutableSamplingIntervalMS: int(report.ExecutableSamplingInterval / time.Millisecond),
					ExecutableSamplingStartedAt:  report.ExecutableSamplingStartedAt,
					ExecutableSamplingStoppedAt:  report.ExecutableSamplingStoppedAt,
					ExecutableObservationCount:   len(report.ExecutableObservations),
					ExecutableObservationErrors:  report.ExecutableObservationErrors,
				}
				lifecycleRecord.AccessEvents = makeLifecycleAccessEvents(report.Events)
				lifecycleRecord.ExecutableObservations = makeLifecycleExecutableObservations(report.ExecutableObservations)
			}
		}
	}
	if runErr != nil {
		fmt.Fprintln(os.Stderr, runErr)
		return 1
	}
	if stopErr != nil {
		fmt.Fprintln(os.Stderr, stopErr)
		return 1
	}
	record.Baseline = baselineReference
	record.Lifecycle = &lifecycleRecord
	if lifecycleRecord.Access != nil {
		record.Assurance.ObservationCoverage = lifecycleObservationCoverage(lifecycleRecord.Access)
		if lifecycleRecord.Access.Status == "captured" {
			record.Assurance.Limitations = append(record.Assurance.Limitations, lifecycleObservationLimitation(lifecycleRecord.Access))
		} else {
			record.Assurance.Limitations = append(record.Assurance.Limitations, "subject executable identity observation was unavailable")
		}
	}
	bundle, err := evidence.WriteBundleWithPolicies(*outputDir, record, sealedPolicy, sealedSubjectPolicy)
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

func runReferences(usingOracle bool, sealed contract.Sealed, artifact oracle.Artifact) (runner.ContractReference, *runner.OracleReference, error) {
	if usingOracle {
		oracleHash, err := oracle.Hash(artifact)
		if err != nil {
			return runner.ContractReference{}, nil, fmt.Errorf("hash oracle for mutation comparison: %w", err)
		}
		return runner.ContractReference{
			ID:      artifact.Contract.ID,
			Version: artifact.Contract.Version,
			SHA256:  artifact.Contract.SHA256,
		}, &runner.OracleReference{Schema: artifact.Schema, SHA256: oracleHash}, nil
	}
	id, ok := sealed.Document.Contract["id"].(string)
	if !ok || id == "" {
		return runner.ContractReference{}, nil, fmt.Errorf("sealed contract ID is missing")
	}
	version, ok := integerValue(sealed.Document.Contract["version"])
	if !ok {
		return runner.ContractReference{}, nil, fmt.Errorf("sealed contract version is invalid")
	}
	return runner.ContractReference{ID: id, Version: version, SHA256: sealed.SHA256}, nil, nil
}

func sealedPolicyHash(sealed *policy.Sealed) string {
	if sealed == nil {
		return ""
	}
	return sealed.SHA256
}

func sameDirectory(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	return leftErr == nil && rightErr == nil && filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
}

func integerValue(value any) (int64, bool) {
	switch typed := value.(type) {
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	case float64:
		converted := int64(typed)
		return converted, float64(converted) == typed
	default:
		return 0, false
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  sorna contract validate <path>")
	fmt.Fprintln(os.Stderr, "  sorna contract seal <path> --output-dir <dir>")
	fmt.Fprintln(os.Stderr, "  sorna policy validate <path>")
	fmt.Fprintln(os.Stderr, "  sorna policy seal <path> --output-dir <dir>")
	fmt.Fprintln(os.Stderr, "  sorna sandbox exec --policy <path> [--root <dir>] -- <command> [args...]")
	fmt.Fprintln(os.Stderr, "  sorna oracle freeze --contract <path> --policy <path> [--root <dir>] [--output-dir <dir>]")
	fmt.Fprintln(os.Stderr, "  sorna mutation validate <catalogue> [--contract <path>]")
	fmt.Fprintln(os.Stderr, "  sorna mutation plan <catalogue> --contract <path> --oracle <path> --baseline-evidence <dir> [--subject-policy <path>] [--output <path>]")
	fmt.Fprintln(os.Stderr, "  sorna mutation provider validate <path>")
	fmt.Fprintln(os.Stderr, "  sorna mutation provider inspect <plan> --provider <path> [--require-plan-binding] [--format text|json|ci-result] [--output <path>]")
	fmt.Fprintln(os.Stderr, "  sorna mutation provider preparation <summary> --provider <path> [--format ci-result] [--source-root <dir>] [--output <path>]")
	fmt.Fprintln(os.Stderr, "  sorna mutation run <plan> --provider <path> --oracle <path> --policy <path> [--subject-policy <path>] [--require-plan-binding] [--base-address <host:port>] [--output-dir <dir>] [--output <path>]")
	fmt.Fprintln(os.Stderr, "  sorna mutation list <catalogue>")
	fmt.Fprintln(os.Stderr, "  sorna evidence verify <directory>")
	fmt.Fprintln(os.Stderr, "  sorna gate [--minimum-observation-coverage <state>] [--format text|json|ci-result] <evidence-directory>")
	fmt.Fprintln(os.Stderr, "  sorna run [--oracle <path> | --contract <path>] [--policy <path>] --base-url <url> [--subject-command <executable> --subject-arg <arg> ...] [--subject-policy <path>] [--baseline-evidence <dir>] [--output-dir <dir>]")
}

type stringList []string

func (list *stringList) String() string {
	return fmt.Sprint([]string(*list))
}

func (list *stringList) Set(value string) error {
	*list = append(*list, value)
	return nil
}
