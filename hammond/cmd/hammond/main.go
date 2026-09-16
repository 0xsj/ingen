package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"ingen/hammond/internal/governance"
	"ingen/hammond/internal/store"
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
	case "register":
		return registerCommand(args[1:])
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
	default:
		fmt.Fprintln(os.Stderr, "unknown Hammond command:", args[0])
		usage()
		return 2
	}
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
	event, err := governance.BuildAmendmentEvent(predecessor, successor, *eventID, *actor, *at, governance.AmendmentKind(*kind), *reason)
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
	event, err := governance.BuildSupersededEvent(predecessor, successor, *eventID, *actor, *at)
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
	if err := governance.ValidateLineage(records); err != nil {
		return printError(err)
	}
	printLineage(records)
	return 0
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
  hammond register --store <dir> --record <path>
  hammond append-event --store <dir> --record <path> --event <path> [--if-revision <revision>]
  hammond amend --store <dir> --record <path> --successor <path> --event-id <id> --actor <actor> --at <RFC3339 UTC> --kind <kind> --reason <text> [--if-revision <revision>]
  hammond supersede --store <dir> --record <path> --successor <path> --event-id <id> --actor <actor> --at <RFC3339 UTC> [--if-revision <revision>]
  hammond show --store <dir> --record <path>
  hammond revision --store <dir> --record <path>
  hammond list --store <dir>
  hammond lineage --store <dir>`
	fmt.Fprintln(os.Stderr, strings.TrimSpace(message))
}
