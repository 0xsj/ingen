package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"time"

	"ingen/core/ciresult"
	sentineladapter "ingen/herdr-sentinel/internal/adapter"
	sentinelaudit "ingen/herdr-sentinel/internal/audit"
	"ingen/herdr-sentinel/internal/capability"
	sentinelreport "ingen/herdr-sentinel/internal/report"
	sentinelrun "ingen/herdr-sentinel/internal/run"
	"ingen/herdr-sentinel/internal/workspace"
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
	case "workspace":
		return workspaceCommand(args[1:])
	case "adapter":
		return adapterCommand(args[1:])
	case "run":
		return runCommand(args[1:])
	default:
		usage()
		return 2
	}
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
	outputPath := flags.String("output", "", "path for the capability plan; stdout when empty")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *workspacePath == "" || len(flags.Args()) != 0 {
		usage()
		return 2
	}
	plan, err := capability.FromFile(*workspacePath)
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
	default:
		usage()
		return 2
	}
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
	plan, err := capability.FromFile(*workspacePath)
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

	plan, err := capability.FromFile(*workspacePath)
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
				if _, statErr := os.Stat(resultPath); statErr == nil {
					if err := receipt.AddFileArtifact("sorna-run", prepared.RoleID, "sorna-run", resultPath); err != nil {
						return false, err
					}
					completionIDs = append(completionIDs, "sorna-run")
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
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	outputPath := filepath.Join(rootAbs, outputDir, name)
	workingDir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	relative, err := filepath.Rel(workingDir, outputPath)
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
			return receipt.RegisterFileArtifact(*id, *role, *kind, *artifactPath)
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
			return receipt.RegisterFileArtifact(*id, *role, *kind, *artifactPath)
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
	changed, err := receipt.RegisterFileArtifact(*id, *role, *kind, *artifactPath)
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
	outputPath := flags.String("output", "", "path for the Sentinel lifecycle receipt")
	runID := flags.String("run-id", "", "optional explicit run ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *workspacePath == "" || *outputPath == "" || len(flags.Args()) != 0 {
		usage()
		return 2
	}
	receipt, err := sentinelrun.New(*workspacePath, time.Now().UTC())
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
	fmt.Fprintln(os.Stderr, "usage: sentinel workspace validate <path>")
	fmt.Fprintln(os.Stderr, "       sentinel workspace capabilities --workspace <path> [--output <path>]")
	fmt.Fprintln(os.Stderr, "       sentinel adapter oracle --workspace <path> [--root <dir>] [--receipt <path>] -- <command> [args...]")
	fmt.Fprintln(os.Stderr, "       sentinel adapter verifier --workspace <path> --oracle <path> --base-url <url> --subject-command <exe> [--subject-arg <arg> ...] [--root <dir>] [--subject-root <dir>] [--ready-path <path>] [--subject-variant <label>] [--output-dir <dir>] [--receipt <path>]")
	fmt.Fprintln(os.Stderr, "       sentinel adapter herdr-event --receipt <path> --event <path> [--root <dir>] [--output <path>]")
	fmt.Fprintln(os.Stderr, "       sentinel adapter herdr-events --receipt <path> --events <path> [--root <dir>] [--output <path>]")
	fmt.Fprintln(os.Stderr, "       sentinel run bootstrap --workspace <path> --output <path>")
	fmt.Fprintln(os.Stderr, "       sentinel run ci-result --receipt <path> [--source-root <dir>] [--output <path>]")
	fmt.Fprintln(os.Stderr, "       sentinel run audit --receipt <path> [--root <dir>] [--output <path>]")
	fmt.Fprintln(os.Stderr, "       sentinel run artifact --receipt <path> --id <id> --role <role> --kind <kind> --path <path> [--output <path>]")
	fmt.Fprintln(os.Stderr, "       sentinel run report --receipt <path> [--root <dir>] [--output <path>]")
}
