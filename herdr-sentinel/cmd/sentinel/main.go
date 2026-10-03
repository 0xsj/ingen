package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ingen/core/ciresult"
	sentineladapter "ingen/herdr-sentinel/internal/adapter"
	sentinelaudit "ingen/herdr-sentinel/internal/audit"
	"ingen/herdr-sentinel/internal/capability"
	sentinelpreflight "ingen/herdr-sentinel/internal/preflight"
	sentinelproject "ingen/herdr-sentinel/internal/project"
	sentinelreport "ingen/herdr-sentinel/internal/report"
	sentinelrun "ingen/herdr-sentinel/internal/run"
	sentinelsession "ingen/herdr-sentinel/internal/session"
	"ingen/herdr-sentinel/internal/workspace"
	sornacontract "ingen/sorna/contract"
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
	case "project":
		return projectCommand(args[1:])
	case "workspace":
		return workspaceCommand(args[1:])
	case "adapter":
		return adapterCommand(args[1:])
	case "run":
		return runCommand(args[1:])
	case "session":
		return sessionCommand(args[1:])
	case "contract":
		return contractCommand(args[1:])
	case "oracle":
		return oracleCommand(args[1:])
	case "verify":
		return verifyCommand(args[1:])
	case "evidence":
		return evidenceCommand(args[1:])
	default:
		usage()
		return 2
	}
}

func sessionCommand(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "spawn":
		return spawnSessionCommand(args[1:])
	case "status":
		return sessionStatusCommand(args[1:])
	case "list":
		return sessionListCommand(args[1:])
	default:
		usage()
		return 2
	}
}

func spawnSessionCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel session spawn", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	workspacePath := flags.String("workspace", "", "Sentinel workspace manifest relative to the project root")
	receiptPath := flags.String("receipt", "", "Sentinel lifecycle receipt relative to the project root")
	roleID := flags.String("role", "", "role ID to launch")
	root := flags.String("root", ".", "project root containing the workspace and receipt")
	outputPath := flags.String("output", "", "session record path relative to the project root")
	stdoutPath := flags.String("stdout", "", "captured stdout path relative to the project root")
	stderrPath := flags.String("stderr", "", "captured stderr path relative to the project root")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *workspacePath == "" || *receiptPath == "" || *roleID == "" || len(flags.Args()) == 0 {
		usage()
		return 2
	}
	record, err := sentinelsession.Spawn(sentinelsession.Request{
		Root:          *root,
		WorkspacePath: *workspacePath,
		ReceiptPath:   *receiptPath,
		RoleID:        *roleID,
		OutputPath:    *outputPath,
		StdoutPath:    *stdoutPath,
		StderrPath:    *stderrPath,
		Command:       flags.Args(),
	})
	if record.SessionID != "" {
		fmt.Println("session:", record.SessionID)
		fmt.Println("status:", record.Status)
	}
	if err == nil {
		return 0
	}
	fmt.Fprintln(os.Stderr, err)
	var exitErr *osexec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

func sessionStatusCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel session status", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("root", ".", "project root containing the session record")
	path := flags.String("path", "", "session record path relative to the project root")
	format := flags.String("format", "text", "output format: text or json")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *path == "" || len(flags.Args()) != 0 || (*format != "text" && *format != "json") {
		usage()
		return 2
	}
	recordPath, err := sentinelrun.ResolveFileRefUnderRoot(*root, ciresult.FileRef{Path: *path})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	record, err := sentinelsession.LoadFile(recordPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *format == "json" {
		if err := sentinelsession.WriteJSON(os.Stdout, record); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}
	fmt.Printf("session: %s\nrun: %s\nrole: %s (%s)\nstatus: %s\nenforcement: %s\nworkspace: %s\n", record.SessionID, record.RunID, record.RoleID, record.RoleKind, record.Status, record.Enforcement, record.Workspace)
	return 0
}

func sessionListCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel session list", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("root", ".", "project root containing session records")
	directory := flags.String("dir", ".ingen/artifacts/sessions", "session record directory relative to the project root")
	format := flags.String("format", "text", "output format: text or json")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if len(flags.Args()) != 0 || (*format != "text" && *format != "json") {
		usage()
		return 2
	}
	directoryPath, err := sentinelrun.ResolveDirectoryUnderRoot(*root, *directory)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	entries, err := os.ReadDir(directoryPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	records := make([]sentinelsession.Record, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		record, loadErr := sentinelsession.LoadFile(filepath.Join(directoryPath, entry.Name()))
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "load session %s: %v\n", entry.Name(), loadErr)
			return 1
		}
		records = append(records, record)
	}
	sort.Slice(records, func(left, right int) bool {
		return records[left].CreatedAt < records[right].CreatedAt
	})
	if *format == "json" {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(records); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}
	for _, record := range records {
		fmt.Printf("%s\t%s\t%s\t%s\n", record.SessionID, record.RoleID, record.Status, record.CreatedAt)
	}
	return 0
}

func contractCommand(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "create":
		return createContractCommand(args[1:])
	case "validate":
		return validateContractCommand(args[1:])
	case "seal":
		return sealContractCommand(args[1:])
	default:
		usage()
		return 2
	}
}

func createContractCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel contract create", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	specPath := flags.String("spec", ".ingen/contract/spec.malc", "Malcolm specification path relative to the project root")
	root := flags.String("root", ".", "fresh project root")
	irPath := flags.String("ir-output", ".ingen/contract/spec.ir.json", "Malcolm IR output path relative to the project root")
	contractPath := flags.String("output", ".ingen/contract/contract.json", "Sorna contract output path relative to the project root")
	ingenRoot := flags.String("ingen-root", ".", "InGen checkout containing malcolm/ and sorna/")
	malcolmManifest := flags.String("malcolm-manifest", "malcolm/Cargo.toml", "Malcolm Cargo manifest relative to the InGen checkout")
	force := flags.Bool("force", false, "replace existing IR and contract outputs")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if len(flags.Args()) != 0 {
		usage()
		return 2
	}
	projectRoot, err := absoluteDirectory(*root, "project")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	toolRoot, err := absoluteDirectory(*ingenRoot, "InGen")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	spec, err := rootedExistingFile(projectRoot, *specPath, "Malcolm specification")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	irOutput, err := rootedOutputFile(projectRoot, *irPath, "Malcolm IR output")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	contractOutput, err := rootedOutputFile(projectRoot, *contractPath, "Sorna contract output")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !*force {
		for name, path := range map[string]string{"Malcolm IR output": irOutput, "Sorna contract output": contractOutput} {
			if _, statErr := os.Stat(path); statErr == nil {
				fmt.Fprintf(os.Stderr, "%s %s already exists; pass --force to replace it\n", name, path)
				return 1
			} else if !os.IsNotExist(statErr) {
				fmt.Fprintf(os.Stderr, "check %s %s: %v\n", name, path, statErr)
				return 1
			}
		}
	}
	if filepath.IsAbs(*malcolmManifest) {
		fmt.Fprintln(os.Stderr, "Malcolm manifest must be relative to the InGen checkout")
		return 1
	}
	manifestPath, err := sentinelrun.ResolveFileRefUnderRoot(toolRoot, ciresult.FileRef{Path: *malcolmManifest})
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve Malcolm manifest: %v\n", err)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(irOutput), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(contractOutput), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	malcolm := osexec.Command("cargo", "run", "--manifest-path", manifestPath, "--", spec, "--output", irOutput)
	malcolm.Dir = toolRoot
	malcolm.Stdout = os.Stdout
	malcolm.Stderr = os.Stderr
	if err := malcolm.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Malcolm failed: %v\n", err)
		return commandExitCode(err)
	}

	bridge := osexec.Command("go", "run", "./sorna/cmd/sorna-malcolm", irOutput, "--output", contractOutput)
	bridge.Dir = toolRoot
	bridge.Stdout = os.Stdout
	bridge.Stderr = os.Stderr
	if err := bridge.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Sorna Malcolm bridge failed: %v\n", err)
		return commandExitCode(err)
	}
	if _, err := sornacontract.LoadFile(contractOutput); err != nil {
		fmt.Fprintf(os.Stderr, "generated contract failed validation: %v\n", err)
		return 1
	}
	fmt.Println("created IR:", filepath.Clean(*irPath))
	fmt.Println("created contract:", filepath.Clean(*contractPath))
	return 0
}

func oracleCommand(args []string) int {
	if len(args) == 0 || args[0] != "freeze" {
		usage()
		return 2
	}
	return freezeOracleCommand(args[1:])
}

func freezeOracleCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel oracle freeze", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("root", ".", "fresh project root")
	contractPath := flags.String("contract", ".ingen/contract/contract.json", "Sorna contract path relative to the project root")
	policyPath := flags.String("policy", ".ingen/policy/oracle.yaml", "Sorna oracle policy path relative to the project root")
	outputDir := flags.String("output-dir", ".ingen/artifacts/oracle", "frozen oracle output directory relative to the project root")
	ingenRoot := flags.String("ingen-root", ".", "InGen checkout containing Sorna")
	force := flags.Bool("force", false, "replace an existing frozen oracle bundle")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if len(flags.Args()) != 0 {
		usage()
		return 2
	}
	projectRoot, err := absoluteDirectory(*root, "project")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	toolRoot, err := absoluteDirectory(*ingenRoot, "InGen")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	contractFile, err := rootedExistingFile(projectRoot, *contractPath, "Sorna contract")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	policyFile, err := rootedExistingFile(projectRoot, *policyPath, "Sorna oracle policy")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	oracleDir, err := rootedOutputDirectory(projectRoot, *outputDir, "frozen oracle output")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !*force {
		oracleFile := filepath.Join(oracleDir, "oracle.json")
		if _, statErr := os.Stat(oracleFile); statErr == nil {
			fmt.Fprintf(os.Stderr, "frozen oracle %s already exists; pass --force to replace it\n", oracleFile)
			return 1
		} else if !os.IsNotExist(statErr) {
			fmt.Fprintf(os.Stderr, "check frozen oracle %s: %v\n", oracleFile, statErr)
			return 1
		}
	}

	return runSornaCommand(toolRoot, "oracle", "freeze",
		"--contract", contractFile,
		"--policy", policyFile,
		"--root", projectRoot,
		"--output-dir", oracleDir,
	)
}

func verifyCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel verify", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("root", ".", "fresh project root")
	ingenRoot := flags.String("ingen-root", ".", "InGen checkout containing Sorna")
	oraclePath := flags.String("oracle", ".ingen/artifacts/oracle/oracle.json", "frozen oracle path relative to the project root")
	policyPath := flags.String("policy", ".ingen/policy/oracle.yaml", "Sorna oracle policy path relative to the project root")
	subjectPolicyPath := flags.String("subject-policy", ".ingen/policy/subject.yaml", "managed-subject policy path relative to the project root")
	subjectRoot := flags.String("subject-root", ".", "subject policy root relative to the project root")
	subjectDir := flags.String("subject-dir", ".", "managed subject working directory relative to the project root")
	baseURL := flags.String("base-url", "http://127.0.0.1:8080", "absolute URL of the running or managed subject")
	readyPath := flags.String("ready-path", "/healthz", "HTTP path that must return 2xx before verification")
	variant := flags.String("subject-variant", "fresh-project", "label for the subject variant")
	subjectCommand := flags.String("subject-command", "", "executable for Sorna to manage")
	outputDir := flags.String("output-dir", ".ingen/artifacts/evidence", "evidence output directory relative to the project root")
	force := flags.Bool("force", false, "allow writing a new run into an output directory containing run.json")
	startupTimeout := flags.Duration("startup-timeout", 10*time.Second, "maximum time to wait for subject readiness")
	shutdownTimeout := flags.Duration("shutdown-timeout", 5*time.Second, "maximum time to wait for graceful subject shutdown")
	var subjectArgs repeatedString
	flags.Var(&subjectArgs, "subject-arg", "argument for the managed subject; may be repeated")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if len(flags.Args()) != 0 || strings.TrimSpace(*subjectCommand) == "" {
		usage()
		return 2
	}
	projectRoot, err := absoluteDirectory(*root, "project")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	toolRoot, err := absoluteDirectory(*ingenRoot, "InGen")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	oracleFile, err := rootedExistingFile(projectRoot, *oraclePath, "frozen oracle")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	policyFile, err := rootedExistingFile(projectRoot, *policyPath, "Sorna oracle policy")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	subjectPolicyFile, err := rootedExistingFile(projectRoot, *subjectPolicyPath, "managed-subject policy")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	subjectRootDir, err := rootedExistingDirectory(projectRoot, *subjectRoot, "subject policy")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	subjectWorkingDir, err := rootedExistingDirectory(projectRoot, *subjectDir, "subject working")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	evidenceDir, err := rootedOutputDirectory(projectRoot, *outputDir, "evidence output")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !*force {
		runFile := filepath.Join(evidenceDir, "run.json")
		if _, statErr := os.Stat(runFile); statErr == nil {
			fmt.Fprintf(os.Stderr, "evidence run %s already exists; choose a new --output-dir or pass --force\n", runFile)
			return 1
		} else if !os.IsNotExist(statErr) {
			fmt.Fprintf(os.Stderr, "check evidence run %s: %v\n", runFile, statErr)
			return 1
		}
	}

	commandArgs := []string{
		"run", "--oracle", oracleFile,
		"--policy", policyFile,
		"--subject-policy", subjectPolicyFile,
		"--subject-root", subjectRootDir,
		"--subject-dir", subjectWorkingDir,
		"--base-url", *baseURL,
		"--ready-path", *readyPath,
		"--subject-variant", *variant,
		"--subject-command", *subjectCommand,
		"--output-dir", evidenceDir,
		"--startup-timeout", startupTimeout.String(),
		"--shutdown-timeout", shutdownTimeout.String(),
	}
	for _, subjectArg := range subjectArgs {
		commandArgs = append(commandArgs, "--subject-arg", subjectArg)
	}
	return runSornaCommand(toolRoot, commandArgs...)
}

func runSornaCommand(toolRoot string, args ...string) int {
	commandArgs := append([]string{"run", "./sorna/cmd/sorna"}, args...)
	command := osexec.Command("go", commandArgs...)
	command.Dir = toolRoot
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Sorna command failed: %v\n", err)
		return commandExitCode(err)
	}
	return 0
}

func evidenceCommand(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "verify":
		return verifyEvidenceCommand(args[1:])
	case "gate":
		return gateEvidenceCommand(args[1:])
	default:
		usage()
		return 2
	}
}

func verifyEvidenceCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel evidence verify", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("root", ".", "fresh project root")
	ingenRoot := flags.String("ingen-root", ".", "InGen checkout containing Sorna")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if len(flags.Args()) != 1 {
		usage()
		return 2
	}
	projectRoot, err := absoluteDirectory(*root, "project")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	toolRoot, err := absoluteDirectory(*ingenRoot, "InGen")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	evidenceDir, err := rootedExistingDirectory(projectRoot, flags.Args()[0], "evidence")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return runSornaCommand(toolRoot, "evidence", "verify", evidenceDir)
}

func gateEvidenceCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel evidence gate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("root", ".", "fresh project root")
	ingenRoot := flags.String("ingen-root", ".", "InGen checkout containing Sorna")
	outputPath := flags.String("output", ".ingen/artifacts/evidence-ci-result.json", "shared CI result output path relative to the project root")
	minimumCoverage := flags.String("minimum-observation-coverage", "", "minimum observation coverage required to pass")
	force := flags.Bool("force", false, "replace an existing CI result")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if len(flags.Args()) != 1 {
		usage()
		return 2
	}
	projectRoot, err := absoluteDirectory(*root, "project")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	toolRoot, err := absoluteDirectory(*ingenRoot, "InGen")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	evidenceDir, err := rootedExistingDirectory(projectRoot, flags.Args()[0], "evidence")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	resultFile, err := rootedOutputFile(projectRoot, *outputPath, "evidence CI result")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !*force {
		if _, statErr := os.Stat(resultFile); statErr == nil {
			fmt.Fprintf(os.Stderr, "evidence CI result %s already exists; pass --force to replace it\n", resultFile)
			return 1
		} else if !os.IsNotExist(statErr) {
			fmt.Fprintf(os.Stderr, "check evidence CI result %s: %v\n", resultFile, statErr)
			return 1
		}
	}
	if err := os.MkdirAll(filepath.Dir(resultFile), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	resultOutput, err := os.OpenFile(resultFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create evidence CI result: %v\n", err)
		return 1
	}
	commandArgs := []string{"run", "./sorna/cmd/sorna", "gate", "--format", "ci-result"}
	if strings.TrimSpace(*minimumCoverage) != "" {
		commandArgs = append(commandArgs, "--minimum-observation-coverage", *minimumCoverage)
	}
	commandArgs = append(commandArgs, evidenceDir)
	command := osexec.Command("go", commandArgs...)
	command.Dir = toolRoot
	command.Stdin = os.Stdin
	command.Stdout = resultOutput
	command.Stderr = os.Stderr
	runErr := command.Run()
	closeErr := resultOutput.Close()
	if runErr != nil {
		fmt.Fprintf(os.Stderr, "Sorna gate failed: %v\n", runErr)
		return commandExitCode(runErr)
	}
	if closeErr != nil {
		fmt.Fprintf(os.Stderr, "close evidence CI result: %v\n", closeErr)
		return 1
	}
	fmt.Println("created evidence CI result:", filepath.Clean(*outputPath))
	return 0
}

func absoluteDirectory(raw, name string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		raw = "."
	}
	path, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("resolve %s root: %w", name, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("read %s root %s: %w", name, path, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s root %s is not a directory", name, path)
	}
	return path, nil
}

func rootedExistingFile(root, raw, name string) (string, error) {
	if filepath.IsAbs(raw) {
		return "", fmt.Errorf("%s path must be relative to the project root: %q", name, raw)
	}
	path, err := sentinelrun.ResolveFileRefUnderRoot(root, ciresult.FileRef{Path: raw})
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", name, err)
	}
	return path, nil
}

func rootedExistingDirectory(root, raw, name string) (string, error) {
	if filepath.IsAbs(raw) {
		return "", fmt.Errorf("%s path must be relative to the project root: %q", name, raw)
	}
	if err := sentinelrun.ValidatePathUnderRoot(root, raw); err != nil {
		return "", fmt.Errorf("validate %s: %w", name, err)
	}
	path := filepath.Join(root, filepath.Clean(raw))
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("read %s directory %s: %w", name, path, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s path %s is not a directory", name, path)
	}
	return path, nil
}

func rootedOutputDirectory(root, raw, name string) (string, error) {
	if filepath.IsAbs(raw) {
		return "", fmt.Errorf("%s path must be relative to the project root: %q", name, raw)
	}
	if err := sentinelrun.ValidatePathUnderRoot(root, raw); err != nil {
		return "", fmt.Errorf("validate %s: %w", name, err)
	}
	return filepath.Join(root, filepath.Clean(raw)), nil
}

func rootedOutputFile(root, raw, name string) (string, error) {
	if filepath.IsAbs(raw) {
		return "", fmt.Errorf("%s path must be relative to the project root: %q", name, raw)
	}
	if err := sentinelrun.ValidatePathUnderRoot(root, raw); err != nil {
		return "", fmt.Errorf("validate %s: %w", name, err)
	}
	return filepath.Join(root, filepath.Clean(raw)), nil
}

func commandExitCode(err error) int {
	var exitErr *osexec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() >= 0 {
		return exitErr.ExitCode()
	}
	return 1
}

func validateContractCommand(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	contractPath := args[0]
	flags := flag.NewFlagSet("sentinel contract validate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("root", ".", "project root containing the contract")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if len(flags.Args()) != 0 {
		usage()
		return 2
	}
	resolvedPath, err := sentinelrun.ResolveFileRefUnderRoot(*root, ciresult.FileRef{Path: contractPath})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if _, err := sornacontract.LoadFile(resolvedPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("valid:", filepath.Clean(args[0]))
	return 0
}

func sealContractCommand(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	contractPath := args[0]
	flags := flag.NewFlagSet("sentinel contract seal", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("root", ".", "project root containing the contract")
	outputDir := flags.String("output-dir", "", "directory for sealed contract artifacts")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if *outputDir == "" || len(flags.Args()) != 0 {
		usage()
		return 2
	}
	resolvedContractPath, err := sentinelrun.ResolveFileRefUnderRoot(*root, ciresult.FileRef{Path: contractPath})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if filepath.IsAbs(*outputDir) {
		fmt.Fprintln(os.Stderr, "contract seal output directory must be relative to the project root")
		return 1
	}
	if err := sentinelrun.ValidateDirectoryPathUnderRoot(*root, *outputDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	rootPath, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	sealed, err := sornacontract.SealFile(resolvedContractPath, filepath.Join(rootPath, filepath.Clean(*outputDir)))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("sealed:", sealed.SHA256)
	return 0
}

func projectCommand(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "init":
		return initProjectCommand(args[1:])
	case "check":
		return checkProjectCommand(args[1:])
	default:
		usage()
		return 2
	}
}

func checkProjectCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel project check", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("root", ".", "project root containing the Sentinel workspace")
	workspacePath := flags.String("workspace", ".ingen/workspace.yaml", "Sentinel workspace manifest relative to the project root")
	format := flags.String("format", "text", "output format: text or json")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if len(flags.Args()) != 0 || (*format != "text" && *format != "json") {
		usage()
		return 2
	}
	result, err := sentinelpreflight.Run(*root, *workspacePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *format == "json" {
		if err := result.WriteJSON(os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	} else if err := result.WriteText(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return result.ExitCode()
}

func initProjectCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel project init", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("root", ".", "new project root")
	id := flags.String("id", "", "stable project and contract namespace")
	implementationRoot := flags.String("implementation-root", "src", "relative implementation root")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *id == "" || len(flags.Args()) != 0 {
		usage()
		return 2
	}
	result, err := sentinelproject.Initialize(sentinelproject.Options{
		Root:               *root,
		ID:                 *id,
		ImplementationRoot: *implementationRoot,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	sort.Strings(result.Files)
	fmt.Println("initialized:", result.Root)
	fmt.Println("workspace:", filepath.Join(result.Root, result.Workspace))
	for _, path := range result.Files {
		fmt.Println("created:", path)
	}
	return 0
}

func workspaceCommand(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "validate":
		return validateWorkspaceCommand(args[1:])
	case "capabilities":
		return capabilitiesCommand(args[1:])
	default:
		usage()
		return 2
	}
}

func validateWorkspaceCommand(args []string) int {
	if len(args) != 1 {
		usage()
		return 2
	}
	flags := flag.NewFlagSet("sentinel workspace validate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if len(flags.Args()) != 1 {
		usage()
		return 2
	}
	if _, err := workspace.LoadFile(flags.Args()[0]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("valid:", filepath.Clean(flags.Args()[0]))
	return 0
}

func capabilitiesCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel workspace capabilities", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	workspacePath := flags.String("workspace", "", "Sentinel workspace manifest")
	root := flags.String("root", ".", "project root containing the workspace manifest and policies")
	outputPath := flags.String("output", "", "path for the capability plan; stdout when empty")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *workspacePath == "" || len(flags.Args()) != 0 {
		usage()
		return 2
	}
	plan, err := capability.FromFileUnderRoot(*root, *workspacePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *outputPath == "" {
		if err := capability.WriteJSON(os.Stdout, plan); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}
	if err := os.MkdirAll(filepath.Dir(*outputPath), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := capability.SaveFile(*outputPath, plan); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("created:", filepath.Clean(*outputPath))
	return 0
}

func adapterCommand(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "oracle":
		return oracleAdapterCommand(args[1:])
	case "verifier":
		return verifierAdapterCommand(args[1:])
	case "herdr-event":
		return herdrEventAdapterCommand(args[1:])
	case "herdr-events":
		return herdrEventsAdapterCommand(args[1:])
	case "herdr-host-envelope":
		return herdrHostEnvelopeCommand(args[1:])
	default:
		usage()
		return 2
	}
}

func herdrHostEnvelopeCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel adapter herdr-host-envelope", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	eventPath := flags.String("event", "", "path to one raw Herdr host event envelope")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *eventPath == "" || len(flags.Args()) != 0 {
		usage()
		return 2
	}
	envelope, err := sentineladapter.LoadHerdrHostEnvelope(*eventPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("valid host envelope:", filepath.Clean(*eventPath))
	fmt.Println("event:", envelope.Event)
	return 0
}

func herdrEventAdapterCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel adapter herdr-event", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	receiptPath := flags.String("receipt", "", "path to the Sentinel lifecycle receipt")
	eventPath := flags.String("event", "", "path to one ingen.herdr-event/v1 JSON event")
	root := flags.String("root", ".", "project root containing receipt artifact references")
	outputPath := flags.String("output", "", "path for the updated receipt; receipt path when empty")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *receiptPath == "" || *eventPath == "" || len(flags.Args()) != 0 {
		usage()
		return 2
	}
	event, err := sentineladapter.LoadHerdrEvent(*eventPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *outputPath == "" {
		appended, err := sentineladapter.ApplyHerdrEventFile(*receiptPath, event, *root)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if appended {
			fmt.Println("appended:", filepath.Clean(*receiptPath))
		} else {
			fmt.Println("unchanged:", filepath.Clean(*receiptPath))
		}
		return 0
	}
	if samePath(*receiptPath, *outputPath) {
		appended, err := sentineladapter.ApplyHerdrEventFile(*receiptPath, event, *root)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if appended {
			fmt.Println("appended:", filepath.Clean(*receiptPath))
		} else {
			fmt.Println("unchanged:", filepath.Clean(*receiptPath))
		}
		return 0
	}
	receipt, err := sentinelrun.LoadFile(*receiptPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	appended, err := sentineladapter.ApplyHerdrEventWithRoot(&receipt, event, *root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if appended {
		if err := os.MkdirAll(filepath.Dir(*outputPath), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := sentinelrun.SaveFile(*outputPath, receipt); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("appended:", filepath.Clean(*outputPath))
		return 0
	}
	fmt.Println("unchanged:", filepath.Clean(*outputPath))
	return 0
}

func herdrEventsAdapterCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel adapter herdr-events", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	receiptPath := flags.String("receipt", "", "path to the Sentinel lifecycle receipt")
	eventsPath := flags.String("events", "", "path to newline-delimited ingen.herdr-event/v1 objects")
	root := flags.String("root", ".", "project root containing receipt artifact references")
	outputPath := flags.String("output", "", "path for the updated receipt; receipt path when empty")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *receiptPath == "" || *eventsPath == "" || len(flags.Args()) != 0 {
		usage()
		return 2
	}
	events, err := sentineladapter.LoadHerdrEventStream(*eventsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *outputPath == "" {
		appended, err := sentineladapter.ApplyHerdrEventsFile(*receiptPath, events, *root)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Printf("appended: %s (%d new events)\n", filepath.Clean(*receiptPath), appended)
		return 0
	}
	if samePath(*receiptPath, *outputPath) {
		appended, err := sentineladapter.ApplyHerdrEventsFile(*receiptPath, events, *root)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Printf("appended: %s (%d new events)\n", filepath.Clean(*receiptPath), appended)
		return 0
	}
	receipt, err := sentinelrun.LoadFile(*receiptPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	appended, err := sentineladapter.ApplyHerdrEventsWithRoot(&receipt, events, *root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(*outputPath), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := sentinelrun.SaveFile(*outputPath, receipt); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("appended: %s (%d new events)\n", filepath.Clean(*outputPath), appended)
	return 0
}

func oracleAdapterCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel adapter oracle", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	workspacePath := flags.String("workspace", "", "Sentinel workspace manifest")
	root := flags.String("root", ".", "project root used by the delegated Sorna command")
	receiptPath := flags.String("receipt", "", "optional Sentinel lifecycle receipt to update")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *workspacePath == "" || len(flags.Args()) == 0 {
		usage()
		return 2
	}
	plan, err := capability.FromFileUnderRoot(*root, *workspacePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	prepared, err := sentineladapter.PrepareOracle(plan, *root, flags.Args(), []string{"go", "run", "./sorna/cmd/sorna"})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if *receiptPath != "" {
		_, err := sentinelrun.UpdateFile(*receiptPath, func(loaded *sentinelrun.Receipt) (bool, error) {
			if loaded.Workspace.ID != plan.Workspace.ID || loaded.Workspace.Version != plan.Workspace.Version || loaded.Workspace.File != plan.Workspace.Manifest {
				return false, fmt.Errorf("Sentinel receipt does not match the capability plan workspace")
			}
			if err := loaded.AddArtifact(sentinelrun.ArtifactRef{
				ID:   "oracle-policy",
				Role: prepared.RoleID,
				Kind: "sorna-policy",
				Ref:  prepared.Policy,
			}); err != nil {
				return false, err
			}
			if err := loaded.SetStatus("running", time.Now().UTC()); err != nil {
				return false, err
			}
			if err := loaded.AppendEvent(sentinelrun.Event{
				Type:        "policy-applied",
				At:          time.Now().UTC().Format(time.RFC3339Nano),
				Role:        prepared.RoleID,
				Workspace:   prepared.RoleWorkspace,
				ArtifactIDs: []string{"oracle-policy"},
				Outcome:     "snapshot-delegated-to-sorna",
			}); err != nil {
				return false, err
			}
			if err := loaded.AppendEvent(sentinelrun.Event{
				Type:        "sorna-started",
				At:          time.Now().UTC().Format(time.RFC3339Nano),
				Role:        prepared.RoleID,
				Workspace:   prepared.RoleWorkspace,
				ArtifactIDs: []string{"oracle-policy"},
				Outcome:     "delegated",
			}); err != nil {
				return false, err
			}
			return true, nil
		})
		if err != nil {
			_ = prepared.Close()
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	fmt.Fprintf(os.Stderr, "adapter: backend=%s enforcement=%s role=%s policy=%s\n", prepared.Backend, prepared.Enforcement, prepared.RoleID, prepared.Policy.SHA256)
	process := osexec.Command(prepared.Command[0], prepared.Command[1:]...)
	process.Dir = *root
	process.Stdin = os.Stdin
	process.Stdout = os.Stdout
	process.Stderr = os.Stderr
	processErr := process.Run()
	if *receiptPath != "" {
		completion := sentinelrun.Event{
			Type:        "sorna-completed",
			At:          time.Now().UTC().Format(time.RFC3339Nano),
			Role:        prepared.RoleID,
			Workspace:   prepared.RoleWorkspace,
			ArtifactIDs: []string{"oracle-policy"},
			Outcome:     "completed",
		}
		finalStatus := "completed"
		if processErr != nil {
			finalStatus = "failed"
			var exitErr *osexec.ExitError
			if errors.As(processErr, &exitErr) {
				completion.Outcome = fmt.Sprintf("exit-code-%d", exitErr.ExitCode())
			} else {
				completion.Outcome = "error"
				completion.Reason = processErr.Error()
			}
		}
		if err := prepared.Close(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		_, err := sentinelrun.UpdateFile(*receiptPath, func(receipt *sentinelrun.Receipt) (bool, error) {
			if err := receipt.AppendEvent(completion); err != nil {
				return false, err
			}
			if err := receipt.SetStatus(finalStatus, time.Now().UTC()); err != nil {
				return false, err
			}
			return true, nil
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	} else if err := prepared.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if processErr != nil {
		var exitErr *osexec.ExitError
		if errors.As(processErr, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintln(os.Stderr, processErr)
		return 1
	}
	return 0
}

func verifierAdapterCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel adapter verifier", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	workspacePath := flags.String("workspace", "", "Sentinel workspace manifest")
	root := flags.String("root", ".", "project root used by the delegated Sorna command")
	oraclePath := flags.String("oracle", "", "frozen oracle artifact")
	baseURL := flags.String("base-url", "", "managed subject base URL")
	subjectRoot := flags.String("subject-root", "", "subject policy root used by Sorna")
	subjectCommand := flags.String("subject-command", "", "managed subject executable")
	readyPath := flags.String("ready-path", "/healthz", "HTTP path used to wait for the subject")
	variant := flags.String("subject-variant", "sentinel-verifier", "subject variant label")
	outputDir := flags.String("output-dir", ".artifacts/sentinel-webhook-verifier", "Sorna evidence output directory")
	receiptPath := flags.String("receipt", "", "optional Sentinel lifecycle receipt to update")
	var subjectArgs repeatedString
	flags.Var(&subjectArgs, "subject-arg", "argument passed to the managed subject; repeatable")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *workspacePath == "" || *oraclePath == "" || *baseURL == "" || *subjectCommand == "" || *outputDir == "" || len(flags.Args()) != 0 {
		usage()
		return 2
	}

	plan, err := capability.FromFileUnderRoot(*root, *workspacePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	prepared, err := sentineladapter.PrepareVerifier(
		plan,
		*root,
		*oraclePath,
		*baseURL,
		*subjectRoot,
		*subjectCommand,
		*readyPath,
		*variant,
		*outputDir,
		[]string(subjectArgs),
		[]string{"go", "run", "./sorna/cmd/sorna"},
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	artifactIDs := []string{"frozen-oracle", "oracle-policy", "subject-policy"}
	if *receiptPath != "" {
		_, err := sentinelrun.UpdateFile(*receiptPath, func(loaded *sentinelrun.Receipt) (bool, error) {
			if loaded.Workspace.ID != plan.Workspace.ID || loaded.Workspace.Version != plan.Workspace.Version || loaded.Workspace.File != plan.Workspace.Manifest {
				return false, fmt.Errorf("Sentinel receipt does not match the capability plan workspace")
			}
			for _, artifact := range []sentinelrun.ArtifactRef{
				{ID: "frozen-oracle", Role: prepared.RoleID, Kind: "sorna-oracle", Ref: prepared.Oracle},
				{ID: "oracle-policy", Role: prepared.RoleID, Kind: "sorna-oracle-policy", Ref: prepared.Policy},
				{ID: "subject-policy", Role: prepared.RoleID, Kind: "sorna-subject-policy", Ref: prepared.SubjectPolicy},
			} {
				if err := loaded.AddArtifact(artifact); err != nil {
					return false, err
				}
			}
			if err := loaded.SetStatus("running", time.Now().UTC()); err != nil {
				return false, err
			}
			if err := loaded.AppendEvent(sentinelrun.Event{
				Type:        "policy-applied",
				At:          time.Now().UTC().Format(time.RFC3339Nano),
				Role:        prepared.RoleID,
				Workspace:   prepared.RoleWorkspace,
				ArtifactIDs: artifactIDs,
				Outcome:     "snapshots-delegated-to-sorna",
			}); err != nil {
				return false, err
			}
			if err := loaded.AppendEvent(sentinelrun.Event{
				Type:        "sorna-started",
				At:          time.Now().UTC().Format(time.RFC3339Nano),
				Role:        prepared.RoleID,
				Workspace:   prepared.RoleWorkspace,
				ArtifactIDs: artifactIDs,
				Outcome:     "delegated",
			}); err != nil {
				return false, err
			}
			return true, nil
		})
		if err != nil {
			_ = prepared.Close()
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	fmt.Fprintf(os.Stderr, "adapter: backend=%s enforcement=%s role=%s oracle=%s policy=%s subject-policy=%s\n", prepared.Backend, prepared.Enforcement, prepared.RoleID, prepared.Oracle.SHA256, prepared.Policy.SHA256, prepared.SubjectPolicy.SHA256)
	process := osexec.Command(prepared.Command[0], prepared.Command[1:]...)
	process.Dir = *root
	process.Stdin = os.Stdin
	process.Stdout = os.Stdout
	process.Stderr = os.Stderr
	processErr := process.Run()
	if *receiptPath != "" {
		finalStatus := "completed"
		completionOutcome := "completed"
		completionReason := ""
		if processErr != nil {
			finalStatus = "failed"
			var exitErr *osexec.ExitError
			if errors.As(processErr, &exitErr) {
				completionOutcome = fmt.Sprintf("exit-code-%d", exitErr.ExitCode())
			} else {
				completionOutcome = "error"
				completionReason = processErr.Error()
			}
		}
		if err := prepared.Close(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		_, err := sentinelrun.UpdateFile(*receiptPath, func(receipt *sentinelrun.Receipt) (bool, error) {
			completionIDs := append([]string(nil), artifactIDs...)
			if resultPath, ok := receiptArtifactPath(*root, *outputDir, "run.json"); ok {
				rootAbs, rootErr := filepath.Abs(*root)
				if rootErr == nil {
					if _, statErr := os.Stat(filepath.Join(rootAbs, resultPath)); statErr == nil {
						if err := receipt.AddFileArtifactUnderRoot("sorna-run", prepared.RoleID, "sorna-run", *root, resultPath); err != nil {
							return false, err
						}
						completionIDs = append(completionIDs, "sorna-run")
					}
				}
			}
			if err := receipt.AppendEvent(sentinelrun.Event{
				Type:        "sorna-completed",
				At:          time.Now().UTC().Format(time.RFC3339Nano),
				Role:        prepared.RoleID,
				Workspace:   prepared.RoleWorkspace,
				ArtifactIDs: completionIDs,
				Outcome:     completionOutcome,
				Reason:      completionReason,
			}); err != nil {
				return false, err
			}
			if err := receipt.SetStatus(finalStatus, time.Now().UTC()); err != nil {
				return false, err
			}
			return true, nil
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	} else if err := prepared.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if processErr != nil {
		var exitErr *osexec.ExitError
		if errors.As(processErr, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintln(os.Stderr, processErr)
		return 1
	}
	return 0
}

type repeatedString []string

func (r *repeatedString) String() string {
	return fmt.Sprint([]string(*r))
}

func (r *repeatedString) Set(value string) error {
	*r = append(*r, value)
	return nil
}

func receiptArtifactPath(root, outputDir, name string) (string, bool) {
	if filepath.IsAbs(outputDir) {
		return "", false
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	outputPath := filepath.Join(rootAbs, outputDir, name)
	relative, err := filepath.Rel(rootAbs, outputPath)
	if err != nil || relative == ".." || filepath.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", false
	}
	return filepath.Clean(relative), true
}

func samePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(filepath.Clean(left))
	rightAbs, rightErr := filepath.Abs(filepath.Clean(right))
	return leftErr == nil && rightErr == nil && leftAbs == rightAbs
}

func runCommand(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "bootstrap":
		return bootstrapRunCommand(args[1:])
	case "status":
		return runStatusCommand(args[1:])
	case "ci-result":
		return ciResultCommand(args[1:])
	case "audit":
		return auditRunCommand(args[1:])
	case "artifact":
		return artifactRunCommand(args[1:])
	case "report":
		return reportRunCommand(args[1:])
	default:
		usage()
		return 2
	}
}

func runStatusCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel run status", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	receiptPath := flags.String("receipt", "", "Sentinel lifecycle receipt relative to the project root")
	root := flags.String("root", ".", "project root containing the receipt")
	status := flags.String("status", "", "new receipt status")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *receiptPath == "" || *status == "" || len(flags.Args()) != 0 {
		usage()
		return 2
	}
	resolvedReceipt, err := sentinelrun.ResolveFileRefUnderRoot(*root, ciresult.FileRef{Path: *receiptPath})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := sentinelrun.ValidateStatus(*status); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	changed, err := sentinelrun.UpdateFile(resolvedReceipt, func(receipt *sentinelrun.Receipt) (bool, error) {
		if err := receipt.SetStatus(*status, time.Now().UTC()); err != nil {
			return false, err
		}
		return true, nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if changed {
		fmt.Printf("status: %s\n", *status)
	} else {
		fmt.Printf("unchanged: %s\n", *status)
	}
	return 0
}

func auditRunCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel run audit", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	receiptPath := flags.String("receipt", "", "path to the Sentinel lifecycle receipt")
	root := flags.String("root", ".", "project root containing receipt file references")
	outputPath := flags.String("output", "", "path for the audit report; stdout when empty")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *receiptPath == "" || len(flags.Args()) != 0 {
		usage()
		return 2
	}
	report, err := sentinelaudit.Build(*receiptPath, *root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *outputPath == "" {
		if err := sentinelaudit.WriteJSON(os.Stdout, report); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(*outputPath), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := sentinelaudit.SaveFile(*outputPath, report); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("created:", filepath.Clean(*outputPath))
	}
	return report.ExitCode()
}

func artifactRunCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel run artifact", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	receiptPath := flags.String("receipt", "", "path to the Sentinel lifecycle receipt")
	root := flags.String("root", ".", "project root containing the artifact file")
	id := flags.String("id", "", "stable artifact ID")
	role := flags.String("role", "", "role that produced the artifact")
	kind := flags.String("kind", "", "producer-owned artifact kind")
	artifactPath := flags.String("path", "", "artifact file path relative to the project root")
	outputPath := flags.String("output", "", "path for the updated receipt; receipt path when empty")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *receiptPath == "" || *id == "" || *role == "" || *kind == "" || *artifactPath == "" || len(flags.Args()) != 0 {
		usage()
		return 2
	}
	if *outputPath == "" {
		changed, err := sentinelrun.UpdateFile(*receiptPath, func(receipt *sentinelrun.Receipt) (bool, error) {
			return receipt.RegisterFileArtifactUnderRoot(*id, *role, *kind, *root, *artifactPath)
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if changed {
			fmt.Println("registered:", filepath.Clean(*receiptPath))
		} else {
			fmt.Println("unchanged:", filepath.Clean(*receiptPath))
		}
		return 0
	}
	if samePath(*receiptPath, *outputPath) {
		changed, err := sentinelrun.UpdateFile(*receiptPath, func(receipt *sentinelrun.Receipt) (bool, error) {
			return receipt.RegisterFileArtifactUnderRoot(*id, *role, *kind, *root, *artifactPath)
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if changed {
			fmt.Println("registered:", filepath.Clean(*receiptPath))
		} else {
			fmt.Println("unchanged:", filepath.Clean(*receiptPath))
		}
		return 0
	}
	receipt, err := sentinelrun.LoadFile(*receiptPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	changed, err := receipt.RegisterFileArtifactUnderRoot(*id, *role, *kind, *root, *artifactPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(*outputPath), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := sentinelrun.SaveFile(*outputPath, receipt); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if changed {
		fmt.Println("registered:", filepath.Clean(*outputPath))
	} else {
		fmt.Println("unchanged:", filepath.Clean(*outputPath))
	}
	return 0
}

func reportRunCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel run report", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	receiptPath := flags.String("receipt", "", "path to the Sentinel lifecycle receipt")
	root := flags.String("root", ".", "project root containing receipt file references")
	outputPath := flags.String("output", "", "path for the operator report; stdout when empty")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *receiptPath == "" || len(flags.Args()) != 0 {
		usage()
		return 2
	}
	document, err := sentinelreport.Build(*receiptPath, *root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *outputPath == "" {
		if err := sentinelreport.Write(os.Stdout, document); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return document.Audit.ExitCode()
	}
	if err := os.MkdirAll(filepath.Dir(*outputPath), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := sentinelreport.SaveFile(*outputPath, document); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("created:", filepath.Clean(*outputPath))
	return document.Audit.ExitCode()
}

func bootstrapRunCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel run bootstrap", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	workspacePath := flags.String("workspace", "", "Sentinel workspace manifest")
	root := flags.String("root", ".", "project root containing the workspace manifest")
	outputPath := flags.String("output", "", "path for the Sentinel lifecycle receipt")
	runID := flags.String("run-id", "", "optional explicit run ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *workspacePath == "" || *outputPath == "" || len(flags.Args()) != 0 {
		usage()
		return 2
	}
	receipt, err := sentinelrun.NewUnderRoot(*root, *workspacePath, time.Now().UTC())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *runID != "" {
		receipt.RunID = *runID
	}
	if err := os.MkdirAll(filepath.Dir(*outputPath), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := sentinelrun.SaveFile(*outputPath, receipt); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("created:", filepath.Clean(*outputPath))
	return 0
}

func ciResultCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel run ci-result", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	receiptPath := flags.String("receipt", "", "path for the Sentinel lifecycle receipt")
	outputPath := flags.String("output", "", "path for the Sentinel CI result; stdout when empty")
	sourceRoot := flags.String("source-root", ".", "source root recorded in the CI result")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *receiptPath == "" || len(flags.Args()) != 0 {
		usage()
		return 2
	}
	receipt, receiptBytes, err := sentinelrun.LoadFileSnapshot(*receiptPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	auditReport, err := sentinelaudit.BuildReceipt(receipt, *sourceRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if auditReport.Status == "failed" {
		fmt.Fprintf(os.Stderr, "Sentinel audit failed: %s\n", auditFailureSummary(auditReport))
		return auditReport.ExitCode()
	}
	auditSummary := sentinelrun.AuditSummary{Status: auditReport.Status}
	for _, check := range auditReport.Checks {
		auditSummary.Checks = append(auditSummary.Checks, sentinelrun.AuditCheck{ID: check.ID, Status: check.Status})
	}
	artifact, err := sentinelrun.BuildCIResultBytes(*receiptPath, receiptBytes, *sourceRoot, auditSummary)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *outputPath == "" {
		if err := ciresult.WriteJSON(os.Stdout, artifact); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(*outputPath), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := ciresult.SaveFile(*outputPath, artifact); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("created:", filepath.Clean(*outputPath))
	}
	return artifact.ExitCode
}

func auditFailureSummary(report sentinelaudit.Report) string {
	for _, check := range report.Checks {
		if check.Status == "failed" {
			return check.Detail
		}
	}
	return "receipt integrity check failed"
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: sentinel project init --root <dir> --id <id> [--implementation-root <dir>]")
	fmt.Fprintln(os.Stderr, "       sentinel project check [--root <dir>] [--workspace <path>] [--format text|json]")
	fmt.Fprintln(os.Stderr, "usage: sentinel workspace validate <path>")
	fmt.Fprintln(os.Stderr, "       sentinel workspace capabilities --workspace <path> [--root <dir>] [--output <path>]")
	fmt.Fprintln(os.Stderr, "       sentinel session spawn --workspace <path> --receipt <path> --role <id> [--root <dir>] [--output <path>] [--stdout <path>] [--stderr <path>] -- <command> [args...]")
	fmt.Fprintln(os.Stderr, "       sentinel session status --path <path> [--root <dir>] [--format text|json]")
	fmt.Fprintln(os.Stderr, "       sentinel session list [--root <dir>] [--dir <path>] [--format text|json]")
	fmt.Fprintln(os.Stderr, "       sentinel contract create [--root <dir>] [--spec <path>] [--ir-output <path>] [--output <path>] [--ingen-root <dir>] [--force]")
	fmt.Fprintln(os.Stderr, "       sentinel contract validate <path> [--root <dir>]")
	fmt.Fprintln(os.Stderr, "       sentinel contract seal <path> --output-dir <dir> [--root <dir>]")
	fmt.Fprintln(os.Stderr, "       sentinel oracle freeze [--root <dir>] [--contract <path>] [--policy <path>] [--output-dir <dir>] [--ingen-root <dir>] [--force]")
	fmt.Fprintln(os.Stderr, "       sentinel verify --subject-command <exe> [--subject-arg <arg> ...] [--root <dir>] [--ingen-root <dir>] [--oracle <path>] [--policy <path>] [--subject-policy <path>] [--base-url <url>] [--ready-path <path>] [--subject-variant <label>] [--output-dir <dir>] [--force]")
	fmt.Fprintln(os.Stderr, "       sentinel evidence verify <directory> [--root <dir>] [--ingen-root <dir>]")
	fmt.Fprintln(os.Stderr, "       sentinel evidence gate <directory> [--root <dir>] [--ingen-root <dir>] [--output <path>] [--minimum-observation-coverage <state>] [--force]")
	fmt.Fprintln(os.Stderr, "       sentinel adapter oracle --workspace <path> [--root <dir>] [--receipt <path>] -- <command> [args...]")
	fmt.Fprintln(os.Stderr, "       sentinel adapter verifier --workspace <path> --oracle <path> --base-url <url> --subject-command <exe> [--subject-arg <arg> ...] [--root <dir>] [--subject-root <dir>] [--ready-path <path>] [--subject-variant <label>] [--output-dir <dir>] [--receipt <path>]")
	fmt.Fprintln(os.Stderr, "       sentinel adapter herdr-event --receipt <path> --event <path> [--root <dir>] [--output <path>]")
	fmt.Fprintln(os.Stderr, "       sentinel adapter herdr-events --receipt <path> --events <path> [--root <dir>] [--output <path>]")
	fmt.Fprintln(os.Stderr, "       sentinel adapter herdr-host-envelope --event <path>")
	fmt.Fprintln(os.Stderr, "       sentinel run bootstrap --workspace <path> [--root <dir>] --output <path>")
	fmt.Fprintln(os.Stderr, "       sentinel run status --receipt <path> --status <status> [--root <dir>]")
	fmt.Fprintln(os.Stderr, "       sentinel run ci-result --receipt <path> [--source-root <dir>] [--output <path>]")
	fmt.Fprintln(os.Stderr, "       sentinel run audit --receipt <path> [--root <dir>] [--output <path>]")
	fmt.Fprintln(os.Stderr, "       sentinel run artifact --receipt <path> [--root <dir>] --id <id> --role <role> --kind <kind> --path <path> [--output <path>]")
	fmt.Fprintln(os.Stderr, "       sentinel run report --receipt <path> [--root <dir>] [--output <path>]")
}
