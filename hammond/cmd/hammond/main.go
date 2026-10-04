package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"ingen/core/ciresult"
	"ingen/core/cliversion"
	publicgovernance "ingen/hammond/governance"
	"ingen/hammond/internal/governance"
	"ingen/hammond/internal/store"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if handled, code := cliversion.Dispatch("hammond", args, os.Stdout, os.Stderr, cliversion.Legacy{}); handled {
		return code
	}
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "register":
		return registerCommand(args[1:])
	case "validate":
		return validateCommand(args[1:])
	case "append-event":
		return appendEventCommand(args[1:])
	case "amend":
		return amendCommand(args[1:])
	case "supersede":
		return supersedeCommand(args[1:])
	case "show":
		return showCommand(args[1:])
	case "revision":
		return revisionCommand(args[1:])
	case "list":
		return listCommand(args[1:])
	case "lineage":
		return lineageCommand(args[1:])
	case "membership-current":
		return membershipCurrentCommand(args[1:])
	case "gate":
		return gateCommand(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "unknown Hammond command:", args[0])
		usage()
		return 2
	}
}

// gateCommand emits a producer-owned CI result after Hammond's read-only
// approval verifier accepts the exact contract and active review-policy bytes.
func gateCommand(args []string) int {
	flags := flag.NewFlagSet("gate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("root", "", "project root for all referenced files")
	approval := flags.String("approval", "", "project-relative Hammond approval record")
	policy := flags.String("review-policy", "", "project-relative active Hammond review policy")
	contract := flags.String("contract", "", "project-relative expected contract artifact")
	projectID := flags.String("project-id", "", "expected contract project ID")
	contractID := flags.String("contract-id", "", "expected contract ID")
	version := flags.Int("contract-version", 0, "expected contract version")
	schema := flags.String("contract-schema", "", "expected contract schema")
	contractSHA := flags.String("contract-sha256", "", "expected lowercase SHA-256 of exact contract bytes")
	output := flags.String("output", "", "project-relative exclusive CI result path")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *root == "" || *approval == "" || *policy == "" || *contract == "" || *projectID == "" || *contractID == "" || *version < 1 || *schema == "" || !validHexDigest(*contractSHA) || *output == "" {
		fmt.Fprintln(os.Stderr, "usage: hammond gate --root ABS --approval REL --review-policy REL --contract REL --project-id ID --contract-id ID --contract-version N --contract-schema SCHEMA --contract-sha256 LOWERHEX --output REL")
		return 2
	}
	if !filepath.IsAbs(*root) {
		fmt.Fprintln(os.Stderr, "hammond gate: --root must be an absolute path")
		return 2
	}
	rootAbs, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	rootAbs, err = filepath.EvalSymlinks(rootAbs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolve project root:", err)
		return 2
	}
	if err := rejectGateOutputOverlap(rootAbs, *output, *approval, *policy, *contract); err != nil {
		fmt.Fprintln(os.Stderr, "hammond gate:", err)
		return 2
	}
	selectedContract, selectedErr := readRooted(rootAbs, *contract)
	if selectedErr != nil || sha256Hex(selectedContract) != *contractSHA {
		if selectedErr == nil {
			selectedErr = fmt.Errorf("selected contract bytes do not match --contract-sha256")
		}
	}
	contractRef := publicgovernance.ContractReference{ProjectID: *projectID, ID: *contractID, Version: *version, Schema: *schema, Artifact: publicgovernance.Artifact{URI: *contract, SHA256: *contractSHA}}
	var verified publicgovernance.ApprovedVerification
	verifyErr := selectedErr
	if verifyErr == nil {
		verified, verifyErr = publicgovernance.VerifyApproved(rootAbs, *approval, *policy, contractRef)
	}
	if verifyErr == nil && filepath.ToSlash(filepath.Clean(filepath.FromSlash(verified.Contract.Artifact.URI))) != *contract {
		verifyErr = fmt.Errorf("selected contract path does not match the contract URI in the approved record")
	}
	var result ciresult.Artifact
	status, exit := "passed", 0
	var report any
	inputs := map[string]ciresult.FileRef{}
	if verifyErr != nil {
		status, exit = "error", 2
		report = map[string]any{"approved": false, "error": verifyErr.Error()}
	} else {
		// Re-read every exact verified path before reporting success. Hammond's
		// verifier returns path/hash snapshots, not the bytes themselves.
		for _, input := range verified.Artifacts {
			contents, readErr := readRooted(rootAbs, input.Path)
			if readErr != nil || sha256Hex(contents) != input.SHA256 {
				if readErr == nil {
					readErr = fmt.Errorf("verified bytes changed after approval verification")
				}
				verifyErr = fmt.Errorf("recheck Hammond input %s: %w", input.Kind, readErr)
				break
			}
			inputs[input.Kind] = ciresult.FileRef{Path: filepath.Join(rootAbs, filepath.FromSlash(input.Path)), SHA256: input.SHA256}
		}
		if verifyErr != nil {
			status, exit = "error", 2
			report = map[string]any{"approved": false, "error": verifyErr.Error()}
			inputs = nil
		} else {
			report = verified
		}
	}
	encodedReport, _ := json.Marshal(report)
	explanationText := "Hammond verified an existing approved contract record against the explicitly selected active review policy and exact contract bytes. This is a governance-state check; it is not behavioral verification, authentication, or attestation."
	if verifyErr != nil {
		explanationText = "Hammond could not verify the requested approved contract under the selected active review policy. This is a governance-state check; it is not behavioral verification, authentication, or attestation."
	}
	explanation, _ := json.Marshal(explanationText)
	result = ciresult.Artifact{Schema: ciresult.Schema, Tool: "hammond", Kind: "approved-contract", Status: status, ExitCode: exit, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Source: ciresult.Source{Root: rootAbs}, Inputs: inputs, Report: encodedReport, Explanation: explanation}
	if verifyErr != nil {
		result.Error = verifyErr.Error()
	}
	if err := result.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, "build Hammond CI result:", err)
		return 2
	}
	if err := writeExclusiveRooted(rootAbs, *output, result); err != nil {
		fmt.Fprintln(os.Stderr, "write Hammond CI result:", err)
		return 2
	}
	if exit != 0 {
		fmt.Fprintln(os.Stderr, verifyErr)
		return exit
	}
	return 0
}

func validHexDigest(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func readRooted(root, relative string) ([]byte, error) {
	clean := filepath.Clean(filepath.FromSlash(relative))
	if relative == "" || filepath.IsAbs(relative) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.ToSlash(clean) != relative {
		return nil, fmt.Errorf("path is not normalized and project-relative")
	}
	h, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer h.Close()
	f, err := h.OpenFile(clean, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("input is not a regular file")
	}
	return io.ReadAll(f)
}

func rejectGateOutputOverlap(root, output string, inputs ...string) error {
	cleanOutput := filepath.Clean(filepath.FromSlash(output))
	if output == "" || filepath.IsAbs(output) || cleanOutput == "." || cleanOutput == ".." || strings.HasPrefix(cleanOutput, ".."+string(filepath.Separator)) || filepath.ToSlash(cleanOutput) != output {
		return fmt.Errorf("output must be a normalized project-relative path")
	}
	if err := rejectOutputSymlinks(root, output); err != nil {
		return err
	}
	for _, input := range inputs {
		cleanInput := filepath.Clean(filepath.FromSlash(input))
		if cleanInput == cleanOutput {
			return fmt.Errorf("output must not overlap selected approval, policy, or contract inputs")
		}
	}
	return nil
}

func rejectOutputSymlinks(root, relative string) error {
	h, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer h.Close()
	current := ""
	for _, component := range strings.Split(filepath.ToSlash(relative), "/") {
		if current == "" {
			current = component
		} else {
			current = filepath.ToSlash(filepath.Join(filepath.FromSlash(current), filepath.FromSlash(component)))
		}
		info, err := h.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect output component %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("output path contains symbolic-link component %s", current)
		}
	}
	return nil
}

func writeExclusiveRooted(root, relative string, value ciresult.Artifact) error {
	clean := filepath.Clean(filepath.FromSlash(relative))
	if relative == "" || filepath.IsAbs(relative) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.ToSlash(clean) != relative {
		return fmt.Errorf("output must be a normalized project-relative path")
	}
	if err := rejectOutputSymlinks(root, relative); err != nil {
		return err
	}
	for _, input := range value.Inputs {
		if input.Path == "" {
			continue
		}
		abs, err := filepath.Abs(input.Path)
		if err != nil {
			return err
		}
		out := filepath.Join(root, clean)
		if abs == out {
			return fmt.Errorf("output overlaps a verified input")
		}
	}
	var encoded bytes.Buffer
	if err := ciresult.WriteJSON(&encoded, value); err != nil {
		return err
	}
	h, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer h.Close()
	dir := filepath.Dir(clean)
	name := filepath.Base(clean)
	temp := filepath.Join(dir, fmt.Sprintf(".%s.tmp-%d-%d", name, os.Getpid(), time.Now().UnixNano()))
	f, err := h.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	removeTemp := true
	defer func() {
		_ = f.Close()
		if removeTemp {
			_ = h.Remove(temp)
		}
	}()
	if _, err := f.Write(encoded.Bytes()); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := h.Link(temp, clean); err != nil {
		return err
	}
	if err := h.Remove(temp); err != nil {
		return err
	}
	removeTemp = false
	directory, err := h.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func registerCommand(args []string) int {
	flags := flag.NewFlagSet("register", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("store", "", "directory for Hammond records")
	recordPath := flags.String("record", "", "JSON file containing a registered record")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *root == "" || *recordPath == "" {
		fmt.Fprintln(os.Stderr, "usage: hammond register --store <dir> --record <path>")
		return 2
	}
	record, err := loadRecord(*recordPath)
	if err != nil {
		return printError(err)
	}
	fileStore, err := store.NewFileStore(*root)
	if err != nil {
		return printError(err)
	}
	if err := fileStore.Register(record); err != nil {
		return printError(err)
	}
	return writeJSON(record)
}

func validateCommand(args []string) int {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	recordPath := flags.String("record", "", "JSON file containing a Hammond record")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *recordPath == "" {
		fmt.Fprintln(os.Stderr, "usage: hammond validate --record <path>")
		return 2
	}
	record, err := loadRecord(*recordPath)
	if err != nil {
		return printError(err)
	}
	return writeJSON(struct {
		Valid    bool                        `json:"valid"`
		RecordID string                      `json:"record_id"`
		Contract governance.ContractIdentity `json:"contract"`
		Policy   governance.PolicyReference  `json:"policy"`
		State    governance.State            `json:"state"`
	}{
		Valid:    true,
		RecordID: record.RecordID,
		Contract: record.Contract.Identity(),
		Policy:   record.Policy,
		State:    record.State,
	})
}

func appendEventCommand(args []string) int {
	flags := flag.NewFlagSet("append-event", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("store", "", "directory for Hammond records")
	recordPath := flags.String("record", "", "JSON file identifying the stored contract")
	eventPath := flags.String("event", "", "JSON file containing one governance event")
	expectedRevision := flags.String("if-revision", "", "append only if the stored record has this revision")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *root == "" || *recordPath == "" || *eventPath == "" {
		fmt.Fprintln(os.Stderr, "usage: hammond append-event --store <dir> --record <path> --event <path>")
		return 2
	}
	record, err := loadRecord(*recordPath)
	if err != nil {
		return printError(err)
	}
	event, err := loadEvent(*eventPath)
	if err != nil {
		return printError(err)
	}
	fileStore, err := store.NewFileStore(*root)
	if err != nil {
		return printError(err)
	}
	var updated governance.Record
	if *expectedRevision == "" {
		updated, err = fileStore.AppendEvent(record.Contract.Identity(), event)
	} else {
		updated, err = fileStore.AppendEventIfRevision(record.Contract.Identity(), *expectedRevision, event)
	}
	if err != nil {
		return printError(err)
	}
	return writeJSON(updated)
}

func amendCommand(args []string) int {
	flags := flag.NewFlagSet("amend", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("store", "", "directory for Hammond records")
	predecessorPath := flags.String("record", "", "JSON file identifying the approved predecessor")
	successorPath := flags.String("successor", "", "JSON file containing the registered successor")
	eventID := flags.String("event-id", "", "stable amendment event ID")
	actor := flags.String("actor", "", "actor creating the amendment")
	at := flags.String("at", "", "RFC3339 UTC amendment timestamp")
	kind := flags.String("kind", "", "amendment kind")
	reason := flags.String("reason", "", "reason for the amendment")
	expectedRevision := flags.String("if-revision", "", "amend only if the predecessor has this revision")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *root == "" || *predecessorPath == "" || *successorPath == "" || *eventID == "" || *actor == "" || *at == "" || *kind == "" || *reason == "" {
		fmt.Fprintln(os.Stderr, "usage: hammond amend --store <dir> --record <path> --successor <path> --event-id <id> --actor <actor> --at <RFC3339 UTC> --kind <kind> --reason <text>")
		return 2
	}
	predecessorReference, err := loadRecord(*predecessorPath)
	if err != nil {
		return printError(err)
	}
	successor, err := loadRecord(*successorPath)
	if err != nil {
		return printError(err)
	}
	fileStore, err := store.NewFileStore(*root)
	if err != nil {
		return printError(err)
	}
	predecessor, err := fileStore.Get(predecessorReference.Contract.Identity())
	if err != nil {
		return printError(err)
	}
	policy, err := governance.LoadReviewPolicy(predecessor.Policy)
	if err != nil {
		return printError(fmt.Errorf("load predecessor review policy: %w", err))
	}
	event, err := governance.BuildAmendmentEventWithPolicy(predecessor, successor, *eventID, *actor, *at, governance.AmendmentKind(*kind), *reason, policy)
	if err != nil {
		return printError(err)
	}
	var updated governance.Record
	if *expectedRevision == "" {
		updated, err = fileStore.CreateAmendment(predecessor.Contract.Identity(), successor, event)
	} else {
		updated, err = fileStore.CreateAmendmentIfRevision(predecessor.Contract.Identity(), *expectedRevision, successor, event)
	}
	if err != nil {
		return printError(err)
	}
	return writeJSON(struct {
		Predecessor governance.Record `json:"predecessor"`
		Successor   governance.Record `json:"successor"`
	}{Predecessor: updated, Successor: successor})
}

func supersedeCommand(args []string) int {
	flags := flag.NewFlagSet("supersede", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("store", "", "directory for Hammond records")
	predecessorPath := flags.String("record", "", "JSON file identifying the predecessor")
	successorPath := flags.String("successor", "", "JSON file identifying the approved successor")
	eventID := flags.String("event-id", "", "stable supersession event ID")
	actor := flags.String("actor", "", "actor superseding the predecessor")
	at := flags.String("at", "", "RFC3339 UTC supersession timestamp")
	expectedRevision := flags.String("if-revision", "", "supersede only if the predecessor has this revision")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *root == "" || *predecessorPath == "" || *successorPath == "" || *eventID == "" || *actor == "" || *at == "" {
		fmt.Fprintln(os.Stderr, "usage: hammond supersede --store <dir> --record <path> --successor <path> --event-id <id> --actor <actor> --at <RFC3339 UTC>")
		return 2
	}
	predecessorReference, err := loadRecord(*predecessorPath)
	if err != nil {
		return printError(err)
	}
	successorReference, err := loadRecord(*successorPath)
	if err != nil {
		return printError(err)
	}
	fileStore, err := store.NewFileStore(*root)
	if err != nil {
		return printError(err)
	}
	predecessor, err := fileStore.Get(predecessorReference.Contract.Identity())
	if err != nil {
		return printError(err)
	}
	successor, err := fileStore.Get(successorReference.Contract.Identity())
	if err != nil {
		return printError(err)
	}
	policy, err := governance.LoadReviewPolicy(predecessor.Policy)
	if err != nil {
		return printError(fmt.Errorf("load predecessor review policy: %w", err))
	}
	event, err := governance.BuildSupersededEventWithPolicy(predecessor, successor, *eventID, *actor, *at, policy)
	if err != nil {
		return printError(err)
	}
	var updated governance.Record
	if *expectedRevision == "" {
		updated, err = fileStore.Supersede(predecessor.Contract.Identity(), successor.Contract.Identity(), event)
	} else {
		updated, err = fileStore.SupersedeIfRevision(predecessor.Contract.Identity(), *expectedRevision, successor.Contract.Identity(), event)
	}
	if err != nil {
		return printError(err)
	}
	return writeJSON(updated)
}

func showCommand(args []string) int {
	flags := flag.NewFlagSet("show", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("store", "", "directory for Hammond records")
	recordPath := flags.String("record", "", "JSON file identifying the stored contract")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *root == "" || *recordPath == "" {
		fmt.Fprintln(os.Stderr, "usage: hammond show --store <dir> --record <path>")
		return 2
	}
	record, err := loadRecord(*recordPath)
	if err != nil {
		return printError(err)
	}
	fileStore, err := store.NewFileStore(*root)
	if err != nil {
		return printError(err)
	}
	stored, err := fileStore.Get(record.Contract.Identity())
	if err != nil {
		return printError(err)
	}
	return writeJSON(stored)
}

func revisionCommand(args []string) int {
	flags := flag.NewFlagSet("revision", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("store", "", "directory for Hammond records")
	recordPath := flags.String("record", "", "JSON file identifying the stored contract")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *root == "" || *recordPath == "" {
		fmt.Fprintln(os.Stderr, "usage: hammond revision --store <dir> --record <path>")
		return 2
	}
	record, err := loadRecord(*recordPath)
	if err != nil {
		return printError(err)
	}
	fileStore, err := store.NewFileStore(*root)
	if err != nil {
		return printError(err)
	}
	stored, err := fileStore.Get(record.Contract.Identity())
	if err != nil {
		return printError(err)
	}
	revision, err := store.RecordRevision(stored)
	if err != nil {
		return printError(fmt.Errorf("calculate Hammond record revision: %w", err))
	}
	return writeJSON(struct {
		Revision string `json:"revision"`
	}{Revision: revision})
}

func listCommand(args []string) int {
	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("store", "", "directory for Hammond records")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *root == "" {
		fmt.Fprintln(os.Stderr, "usage: hammond list --store <dir>")
		return 2
	}
	fileStore, err := store.NewFileStore(*root)
	if err != nil {
		return printError(err)
	}
	records, err := fileStore.List()
	if err != nil {
		return printError(err)
	}
	return writeJSON(records)
}

func lineageCommand(args []string) int {
	flags := flag.NewFlagSet("lineage", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("store", "", "directory for Hammond records")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *root == "" {
		fmt.Fprintln(os.Stderr, "usage: hammond lineage --store <dir>")
		return 2
	}
	fileStore, err := store.NewFileStore(*root)
	if err != nil {
		return printError(err)
	}
	records, err := fileStore.List()
	if err != nil {
		return printError(err)
	}
	if err := governance.ValidateLineageWithPolicyResolver(records, governance.LoadReviewPolicy); err != nil {
		return printError(err)
	}
	printLineage(records)
	return 0
}

func membershipCurrentCommand(args []string) int {
	flags := flag.NewFlagSet("membership-current", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("store", "", "directory for Hammond membership versions")
	id := flags.String("id", "", "membership identity to inspect")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *root == "" || *id == "" {
		fmt.Fprintln(os.Stderr, "usage: hammond membership-current --store <dir> --id <membership-id>")
		return 2
	}
	versionStore, err := store.NewFileMembershipVersionStore(*root)
	if err != nil {
		return printError(err)
	}
	reference, err := versionStore.Current(*id)
	if err != nil {
		return printError(err)
	}
	return writeJSON(reference)
}

func loadRecord(path string) (governance.Record, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return governance.Record{}, fmt.Errorf("read Hammond record %s: %w", path, err)
	}
	record, err := governance.DecodeRecord(contents)
	if err != nil {
		return governance.Record{}, fmt.Errorf("parse Hammond record %s: %w", path, err)
	}
	return record, nil
}

func loadEvent(path string) (governance.Event, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return governance.Event{}, fmt.Errorf("read Hammond event %s: %w", path, err)
	}
	event, err := governance.DecodeEvent(contents)
	if err != nil {
		return governance.Event{}, fmt.Errorf("parse Hammond event %s: %w", path, err)
	}
	return event, nil
}

func writeJSON(value any) int {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return printError(err)
	}
	return 0
}

func printLineage(records []governance.Record) {
	seenEdges := make(map[string]struct{})
	for _, record := range records {
		identity := record.Contract.Identity()
		fmt.Printf("%s/%s@%d state=%s sha256=%s\n", identity.ProjectID, identity.ID, identity.Version, record.State, identity.ArtifactSHA256)
		for _, event := range record.Events {
			var successor *governance.ContractIdentity
			switch event.Type {
			case governance.EventAmendmentCreated, governance.EventSuperseded:
				successor = event.Successor
			}
			if successor == nil {
				continue
			}
			edge := identity.Key() + "->" + successor.Key()
			if _, exists := seenEdges[edge]; exists {
				continue
			}
			seenEdges[edge] = struct{}{}
			fmt.Printf("  -> %s/%s@%d sha256=%s\n", successor.ProjectID, successor.ID, successor.Version, successor.ArtifactSHA256)
		}
	}
}

func printError(err error) int {
	fmt.Fprintln(os.Stderr, err)
	return 1
}

func usage() {
	message := `usage:
  hammond [--version | version [--format text|json]]
  hammond register --store <dir> --record <path>
  hammond validate --record <path>
  hammond append-event --store <dir> --record <path> --event <path> [--if-revision <revision>]
  hammond amend --store <dir> --record <path> --successor <path> --event-id <id> --actor <actor> --at <RFC3339 UTC> --kind <kind> --reason <text> [--if-revision <revision>]
  hammond supersede --store <dir> --record <path> --successor <path> --event-id <id> --actor <actor> --at <RFC3339 UTC> [--if-revision <revision>]
  hammond show --store <dir> --record <path>
  hammond revision --store <dir> --record <path>
  hammond list --store <dir>
  hammond lineage --store <dir>
  hammond gate --root <project> --approval <rel> --review-policy <rel> --contract <rel> --project-id <id> --contract-id <id> --contract-version <n> --contract-schema <schema> --contract-sha256 <lowercase-hex> --output <rel>`
	fmt.Fprintln(os.Stderr, strings.TrimSpace(message))
}
