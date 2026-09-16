package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	ciresultadapter "ingen/lockwood/internal/adapters/ciresult"
	"ingen/lockwood/internal/adapters/sorna"
	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/attestation"
	"ingen/lockwood/internal/catalog"
	"ingen/lockwood/internal/custody"
	"ingen/lockwood/internal/integrity"
	"ingen/lockwood/internal/store"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	switch args[0] {
	case "put":
		return runPut(args[1:], stdin, stdout, stderr)
	case "import-sorna":
		return runImportSorna(args[1:], stdout, stderr)
	case "import-ci-result":
		return runImportCIResult(args[1:], stdout, stderr)
	case "get":
		return runGet(args[1:], stdout, stderr)
	case "inspect":
		return runInspect(args[1:], stdout, stderr)
	case "lineage-status":
		return runLineageStatus(args[1:], stdout, stderr)
	case "append-event":
		return runAppendEvent(args[1:], stdout, stderr)
	case "list-events":
		return runListEvents(args[1:], stdout, stderr)
	case "handling-status":
		return runHandlingStatus(args[1:], stdout, stderr)
	case "redaction-status":
		return runRedactionStatus(args[1:], stdout, stderr)
	case "check-handling-guard":
		return runCheckHandlingGuard(args[1:], stdout, stderr)
	case "register-redaction":
		return runRegisterRedaction(args[1:], stdout, stderr)
	case "promote-redaction":
		return runPromoteRedaction(args[1:], stdout, stderr)
	case "sign-redaction-provenance":
		return runSignRedactionProvenance(args[1:], stdout, stderr)
	case "inspect-attestation":
		return runInspectAttestation(args[1:], stdout, stderr)
	case "inspect-attestation-link":
		return runInspectAttestationLink(args[1:], stdout, stderr)
	case "inspect-redaction-provenance":
		return runInspectRedactionProvenance(args[1:], stdout, stderr)
	case "inspect-redaction-provenance-link":
		return runInspectRedactionProvenanceLink(args[1:], stdout, stderr)
	case "record-digest":
		return runRecordDigest(args[1:], stdout, stderr)
	case "find-attestation":
		return runFindAttestation(args[1:], stdout, stderr)
	case "find-redaction-provenance":
		return runFindRedactionProvenance(args[1:], stdout, stderr)
	case "find-trusted-attestation":
		return runFindTrustedAttestation(args[1:], stdout, stderr)
	case "find-trusted-redaction-provenance":
		return runFindTrustedRedactionProvenance(args[1:], stdout, stderr)
	case "sign-attestation":
		return runSignAttestation(args[1:], stdout, stderr)
	case "sign-handling-event":
		return runSignHandlingEvent(args[1:], stdout, stderr)
	case "import-attestation":
		return runImportAttestation(args[1:], stdout, stderr)
	case "import-redaction-provenance":
		return runImportRedactionProvenance(args[1:], stdout, stderr)
	case "verify-attestation":
		return runVerifyAttestation(args[1:], stdout, stderr)
	case "verify-attestation-trusted":
		return runVerifyAttestationTrusted(args[1:], stdout, stderr)
	case "verify-handling-event":
		return runVerifyHandlingEvent(args[1:], stdout, stderr)
	case "verify-handling-event-trusted":
		return runVerifyHandlingEventTrusted(args[1:], stdout, stderr)
	case "verify-handling-event-authorized":
		return runVerifyHandlingEventAuthorized(args[1:], stdout, stderr)
	case "verify-redaction-provenance":
		return runVerifyRedactionProvenance(args[1:], stdout, stderr)
	case "verify-redaction-provenance-trusted":
		return runVerifyRedactionProvenanceTrusted(args[1:], stdout, stderr)
	case "verify":
		return runVerify(args[1:], stdout, stderr)
	case "find":
		return runFind(args[1:], stdout, stderr)
	case "recover":
		return runRecover(args[1:], stdout, stderr)
	case "reconcile":
		return runReconcile(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		usage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		usage(stderr)
		return 2
	}
}

func runPut(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood put", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	schema := flags.String("schema", custody.SchemaV1, "custody record schema")
	custodyID := flags.String("id", "", "custody record ID")
	mediaType := flags.String("media-type", "", "artifact media type")
	logicalName := flags.String("name", "", "artifact logical name")
	producer := flags.String("producer", "", "producing tool")
	kind := flags.String("kind", "", "producer artifact kind")
	runID := flags.String("run-id", "", "producer run ID")
	sourcePath := flags.String("source-path", "", "source path recorded in custody")
	sourceURI := flags.String("source-uri", "", "remote source URI recorded in custody")
	sourceVersion := flags.String("source-version", "", "remote source version or object version")
	expectedDigest := flags.String("expected-digest", "", "optional expected SHA-256 digest")
	receivedAt := flags.String("received-at", "", "RFC3339 receipt time")
	retentionClass := flags.String("retention-class", "default", "retention class")
	redaction := flags.String("redaction", "none", "redaction status")
	maxBytes := flags.Int64("max-bytes", 0, "maximum artifact size in bytes; 0 means unlimited")
	pendingRecord := flags.String("pending-record", "", "write a recoverable pending record if publication fails")
	var parents lineageFlags
	flags.Var(&parents, "parent", "lineage parent as relation=digest; repeatable")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "put requires exactly one input path, or - for stdin")
		return 2
	}
	if *root == "" || *custodyID == "" || *mediaType == "" || *producer == "" || *kind == "" {
		fmt.Fprintln(stderr, "put requires --root, --id, --media-type, --producer, and --kind")
		return 2
	}

	parsedReceivedAt, err := parseTime(*receivedAt)
	if err != nil {
		fmt.Fprintf(stderr, "invalid --received-at: %v\n", err)
		return 2
	}
	inputPath := flags.Arg(0)
	input := stdin
	var file *os.File
	if inputPath != "-" {
		file, err = os.Open(inputPath)
		if err != nil {
			fmt.Fprintf(stderr, "open input: %v\n", err)
			return 1
		}
		defer file.Close()
		input = file
	}
	if *logicalName == "" {
		if inputPath == "-" {
			*logicalName = "stdin"
		} else {
			*logicalName = filepath.Base(inputPath)
		}
	}
	if *sourcePath == "" {
		*sourcePath = inputPath
	}

	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	ingestor, err := custody.NewIngestor(artifacts, records)
	if err != nil {
		fmt.Fprintf(stderr, "create ingestor: %v\n", err)
		return 1
	}
	record, err := ingestor.Accept(input, custody.IntakeRequest{
		Schema:         *schema,
		CustodyID:      *custodyID,
		ExpectedDigest: *expectedDigest,
		MediaType:      *mediaType,
		LogicalName:    *logicalName,
		MaxBytes:       *maxBytes,
		ReceivedAt:     parsedReceivedAt,
		Producer:       custody.Producer{Tool: *producer, Kind: *kind},
		Source:         custody.Source{RunID: *runID, Path: *sourcePath, URI: *sourceURI, Version: *sourceVersion},
		Parents:        []custody.Lineage(parents),
		Handling:       custody.Handling{Redaction: *redaction, RetentionClass: *retentionClass},
	})
	if err != nil {
		saved, saveErr := savePendingRecord(*pendingRecord, err)
		if saveErr != nil {
			fmt.Fprintf(stderr, "put: %v (save pending record: %v)\n", err, saveErr)
			return 1
		}
		if saved {
			fmt.Fprintf(stderr, "put: %v (pending record saved to %s)\n", err, *pendingRecord)
			return 1
		}
		fmt.Fprintf(stderr, "put: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, record); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runGet(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood get", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	output := flags.String("output", "-", "output path, or - for stdout")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 || *root == "" {
		fmt.Fprintln(stderr, "get requires --root and exactly one digest")
		return 2
	}
	artifacts, _, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	data, err := artifacts.Get(flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "get: %v\n", err)
		return 1
	}
	if *output == "-" {
		if _, err := stdout.Write(data); err != nil {
			fmt.Fprintf(stderr, "write output: %v\n", err)
			return 1
		}
		return 0
	}
	file, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		fmt.Fprintf(stderr, "create output: %v\n", err)
		return 1
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		fmt.Fprintf(stderr, "write output: %v\n", err)
		return 1
	}
	if err := file.Close(); err != nil {
		fmt.Fprintf(stderr, "close output: %v\n", err)
		return 1
	}
	return 0
}

func runImportSorna(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood import-sorna", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	custodyID := flags.String("id", "", "custody record ID")
	logicalName := flags.String("name", "", "archive logical name")
	expectedDigest := flags.String("expected-digest", "", "optional expected SHA-256 digest")
	receivedAt := flags.String("received-at", "", "RFC3339 receipt time")
	retentionClass := flags.String("retention-class", "default", "retention class")
	redaction := flags.String("redaction", "none", "redaction status")
	maxBytes := flags.Int64("max-bytes", 0, "maximum archive size in bytes; 0 means unlimited")
	pendingRecord := flags.String("pending-record", "", "write a recoverable pending record if publication fails")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 || *root == "" || *custodyID == "" {
		fmt.Fprintln(stderr, "import-sorna requires --root, --id, and exactly one bundle directory")
		return 2
	}
	parsedReceivedAt, err := parseTime(*receivedAt)
	if err != nil {
		fmt.Fprintf(stderr, "invalid --received-at: %v\n", err)
		return 2
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	ingestor, err := custody.NewIngestor(artifacts, records)
	if err != nil {
		fmt.Fprintf(stderr, "create ingestor: %v\n", err)
		return 1
	}
	importer, err := sorna.NewImporter(ingestor)
	if err != nil {
		fmt.Fprintf(stderr, "create Sorna importer: %v\n", err)
		return 1
	}
	record, err := importer.Import(flags.Arg(0), sorna.ImportRequest{
		CustodyID:      *custodyID,
		ExpectedDigest: *expectedDigest,
		LogicalName:    *logicalName,
		MaxBytes:       *maxBytes,
		ReceivedAt:     parsedReceivedAt,
		RetentionClass: *retentionClass,
		Redaction:      *redaction,
	})
	if err != nil {
		saved, saveErr := savePendingRecord(*pendingRecord, err)
		if saveErr != nil {
			fmt.Fprintf(stderr, "import-sorna: %v (save pending record: %v)\n", err, saveErr)
			return 1
		}
		if saved {
			fmt.Fprintf(stderr, "import-sorna: %v (pending record saved to %s)\n", err, *pendingRecord)
			return 1
		}
		fmt.Fprintf(stderr, "import-sorna: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, record); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runImportCIResult(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood import-ci-result", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	schema := flags.String("schema", custody.SchemaV1, "custody record schema")
	custodyID := flags.String("id", "", "custody record ID")
	logicalName := flags.String("name", "", "artifact logical name")
	expectedDigest := flags.String("expected-digest", "", "optional expected SHA-256 digest")
	receivedAt := flags.String("received-at", "", "RFC3339 receipt time")
	retentionClass := flags.String("retention-class", "default", "retention class")
	redaction := flags.String("redaction", "none", "redaction status")
	maxBytes := flags.Int64("max-bytes", 0, "maximum artifact size in bytes; 0 means unlimited")
	pendingRecord := flags.String("pending-record", "", "write a recoverable pending record if publication fails")
	sourceURI := flags.String("source-uri", "", "remote source URI")
	sourceVersion := flags.String("source-version", "", "remote source version or object version")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 || *root == "" || *custodyID == "" {
		fmt.Fprintln(stderr, "import-ci-result requires --root, --id, and exactly one result path")
		return 2
	}
	parsedReceivedAt, err := parseTime(*receivedAt)
	if err != nil {
		fmt.Fprintf(stderr, "invalid --received-at: %v\n", err)
		return 2
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	ingestor, err := custody.NewIngestor(artifacts, records)
	if err != nil {
		fmt.Fprintf(stderr, "create ingestor: %v\n", err)
		return 1
	}
	importer, err := ciresultadapter.NewImporter(ingestor)
	if err != nil {
		fmt.Fprintf(stderr, "create CI-result importer: %v\n", err)
		return 1
	}
	record, err := importer.Import(flags.Arg(0), ciresultadapter.ImportRequest{
		Schema:         *schema,
		CustodyID:      *custodyID,
		ExpectedDigest: *expectedDigest,
		LogicalName:    *logicalName,
		MaxBytes:       *maxBytes,
		ReceivedAt:     parsedReceivedAt,
		Source:         custody.Source{Path: filepath.Clean(flags.Arg(0)), URI: *sourceURI, Version: *sourceVersion},
		RetentionClass: *retentionClass,
		Redaction:      *redaction,
	})
	if err != nil {
		saved, saveErr := savePendingRecord(*pendingRecord, err)
		if saveErr != nil {
			fmt.Fprintf(stderr, "import-ci-result: %v (save pending record: %v)\n", err, saveErr)
			return 1
		}
		if saved {
			fmt.Fprintf(stderr, "import-ci-result: %v (pending record saved to %s)\n", err, *pendingRecord)
			return 1
		}
		fmt.Fprintf(stderr, "import-ci-result: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, record); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runInspect(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood inspect", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 || *root == "" {
		fmt.Fprintln(stderr, "inspect requires --root and exactly one custody ID")
		return 2
	}
	_, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	record, err := records.Get(flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "inspect: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, record); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runLineageStatus(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood lineage-status", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 || *root == "" {
		fmt.Fprintln(stderr, "lineage-status requires --root and exactly one custody ID")
		return 2
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	record, err := records.Get(flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "lineage-status: %v\n", err)
		return 1
	}
	report, err := custody.AnalyzeLineage(records, artifacts, record)
	if err != nil {
		fmt.Fprintf(stderr, "lineage-status: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, report); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runAppendEvent(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood append-event", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	custodyID := flags.String("id", "", "target custody record ID")
	eventID := flags.String("event-id", "", "immutable handling event ID")
	eventType := flags.String("type", "", "handling event type")
	recordedAt := flags.String("recorded-at", "", "RFC3339 event time")
	actor := flags.String("actor", "", "descriptive actor identity")
	reason := flags.String("reason", "", "reason recorded for the event")
	originalDigest := flags.String("original-digest", "", "original artifact digest for a redaction event")
	resultingDigest := flags.String("resulting-digest", "", "resulting artifact digest for a redaction event")
	retentionClass := flags.String("retention-class", "", "retention class for a classification event")
	legalHoldID := flags.String("legal-hold-id", "", "legal hold ID for a legal-hold event")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *root == "" || *custodyID == "" || *eventID == "" || *eventType == "" || *recordedAt == "" || *actor == "" || *reason == "" {
		fmt.Fprintln(stderr, "append-event requires --root, --id, --event-id, --type, --recorded-at, --actor, --reason, and no positional arguments")
		return 2
	}
	parsedRecordedAt, err := parseTime(*recordedAt)
	if err != nil {
		fmt.Fprintf(stderr, "invalid --recorded-at: %v\n", err)
		return 2
	}
	_, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	if _, err := records.Get(*custodyID); err != nil {
		fmt.Fprintf(stderr, "append-event: read custody record: %v\n", err)
		return 1
	}
	event := custody.HandlingEvent{
		Schema:          custody.HandlingEventSchema,
		EventID:         *eventID,
		CustodyID:       *custodyID,
		Type:            custody.HandlingEventType(*eventType),
		RecordedAt:      parsedRecordedAt,
		Actor:           *actor,
		Reason:          *reason,
		OriginalDigest:  *originalDigest,
		ResultingDigest: *resultingDigest,
		RetentionClass:  *retentionClass,
		LegalHoldID:     *legalHoldID,
	}
	if err := records.AppendEvent(event); err != nil {
		fmt.Fprintf(stderr, "append-event: %v\n", err)
		return 1
	}
	stored, err := records.GetEvent(*custodyID, *eventID)
	if err != nil {
		fmt.Fprintf(stderr, "append-event: read published event: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, stored); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runListEvents(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood list-events", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	custodyID := flags.String("id", "", "target custody record ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *root == "" || *custodyID == "" {
		fmt.Fprintln(stderr, "list-events requires --root, --id, and no positional arguments")
		return 2
	}
	_, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	if _, err := records.Get(*custodyID); err != nil {
		fmt.Fprintf(stderr, "list-events: read custody record: %v\n", err)
		return 1
	}
	events, err := records.ListEvents(*custodyID)
	if err != nil {
		fmt.Fprintf(stderr, "list-events: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, events); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runHandlingStatus(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood handling-status", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	custodyID := flags.String("id", "", "target custody record ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *root == "" || *custodyID == "" {
		fmt.Fprintln(stderr, "handling-status requires --root, --id, and no positional arguments")
		return 2
	}
	_, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	record, err := records.Get(*custodyID)
	if err != nil {
		fmt.Fprintf(stderr, "handling-status: read custody record: %v\n", err)
		return 1
	}
	status, err := custody.AnalyzeHandling(records, records, record)
	if err != nil {
		fmt.Fprintf(stderr, "handling-status: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, status); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runRedactionStatus(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood redaction-status", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	sourceID := flags.String("source-id", "", "custody record containing the redaction event")
	eventID := flags.String("event-id", "", "redaction event ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *root == "" || *sourceID == "" || *eventID == "" {
		fmt.Fprintln(stderr, "redaction-status requires --root, --source-id, --event-id, and no positional arguments")
		return 2
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	status, err := custody.AnalyzeRedaction(records, artifacts, records, *sourceID, *eventID)
	if err != nil {
		fmt.Fprintf(stderr, "redaction-status: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, status); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runCheckHandlingGuard(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood check-handling-guard", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	custodyID := flags.String("id", "", "target custody record ID")
	action := flags.String("action", "", "payload-changing action: redact or delete")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *root == "" || *custodyID == "" || *action == "" {
		fmt.Fprintln(stderr, "check-handling-guard requires --root, --id, --action, and no positional arguments")
		return 2
	}
	_, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	record, err := records.Get(*custodyID)
	if err != nil {
		fmt.Fprintf(stderr, "check-handling-guard: read custody record: %v\n", err)
		return 1
	}
	decision, err := custody.EvaluateHandlingGuard(records, records, record, custody.HandlingAction(*action))
	if err != nil {
		fmt.Fprintf(stderr, "check-handling-guard: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, decision); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	if decision.Status == custody.HandlingBlocked {
		return 1
	}
	return 0
}

func runRegisterRedaction(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood register-redaction", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	custodyID := flags.String("id", "", "target custody record ID")
	eventID := flags.String("event-id", "", "immutable redaction event ID")
	recordedAt := flags.String("recorded-at", "", "RFC3339 redaction event time")
	actor := flags.String("actor", "", "descriptive actor identity")
	reason := flags.String("reason", "", "reason recorded for the redaction")
	originalDigest := flags.String("original-digest", "", "digest of the existing artifact being redacted")
	resultingDigest := flags.String("resulting-digest", "", "digest of a caller-produced resulting artifact already in Lockwood")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *root == "" || *custodyID == "" || *eventID == "" || *recordedAt == "" || *actor == "" || *reason == "" || *originalDigest == "" || *resultingDigest == "" {
		fmt.Fprintln(stderr, "register-redaction requires --root, --id, --event-id, --recorded-at, --actor, --reason, --original-digest, --resulting-digest, and no positional arguments")
		return 2
	}
	parsedRecordedAt, err := parseTime(*recordedAt)
	if err != nil {
		fmt.Fprintf(stderr, "invalid --recorded-at: %v\n", err)
		return 2
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	event, err := custody.RegisterRedaction(artifacts, records, records, custody.HandlingEvent{
		Schema:          custody.HandlingEventSchema,
		EventID:         *eventID,
		CustodyID:       *custodyID,
		Type:            custody.RedactionEvent,
		RecordedAt:      parsedRecordedAt,
		Actor:           *actor,
		Reason:          *reason,
		OriginalDigest:  *originalDigest,
		ResultingDigest: *resultingDigest,
	})
	if err != nil {
		fmt.Fprintf(stderr, "register-redaction: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, event); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runPromoteRedaction(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood promote-redaction", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	sourceID := flags.String("source-id", "", "custody record containing the redaction event")
	eventID := flags.String("event-id", "", "registered redaction event ID")
	custodyID := flags.String("id", "", "new custody record ID")
	resultingDigest := flags.String("resulting-digest", "", "digest of the registered redaction result")
	resultingSize := flags.Int64("size-bytes", -1, "resulting artifact size in bytes")
	mediaType := flags.String("media-type", "", "resulting artifact media type")
	logicalName := flags.String("name", "", "resulting artifact logical name")
	schema := flags.String("schema", "", "custody record schema; defaults to the source schema")
	receivedAt := flags.String("received-at", "", "RFC3339 promotion receipt time")
	producer := flags.String("producer", "", "producing tool for the promoted artifact")
	kind := flags.String("kind", "", "produced artifact kind")
	version := flags.String("producer-version", "", "producing tool version")
	runID := flags.String("run-id", "", "producer run ID")
	sourcePath := flags.String("source-path", "", "source path recorded in the promoted custody record")
	sourceURI := flags.String("source-uri", "", "remote source URI recorded in the promoted custody record")
	sourceVersion := flags.String("source-version", "", "remote source version or object version")
	retentionClass := flags.String("retention-class", "default", "retention class")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *root == "" || *sourceID == "" || *eventID == "" || *custodyID == "" || *resultingDigest == "" || *resultingSize < 0 || *mediaType == "" || *receivedAt == "" || *producer == "" || *kind == "" {
		fmt.Fprintln(stderr, "promote-redaction requires --root, --source-id, --event-id, --id, --resulting-digest, --size-bytes, --media-type, --received-at, --producer, --kind, and no positional arguments")
		return 2
	}
	parsedReceivedAt, err := parseTime(*receivedAt)
	if err != nil {
		fmt.Fprintf(stderr, "invalid --received-at: %v\n", err)
		return 2
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	record, err := custody.PromoteRedactionResult(artifacts, records, records, *sourceID, *eventID, custody.RedactionPromotionRequest{
		Schema:    *schema,
		CustodyID: *custodyID,
		Artifact: artifact.Reference{
			Schema:      artifact.Schema,
			Digest:      *resultingDigest,
			SizeBytes:   *resultingSize,
			MediaType:   *mediaType,
			LogicalName: *logicalName,
		},
		ReceivedAt: parsedReceivedAt,
		Producer:   custody.Producer{Tool: *producer, Kind: *kind, Version: *version},
		Source:     custody.Source{RunID: *runID, Path: *sourcePath, URI: *sourceURI, Version: *sourceVersion},
		Handling:   custody.Handling{Redaction: "redacted", RetentionClass: *retentionClass},
	})
	if err != nil {
		fmt.Fprintf(stderr, "promote-redaction: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, record); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runInspectAttestation(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood inspect-attestation", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 || *root == "" {
		fmt.Fprintln(stderr, "inspect-attestation requires --root and exactly one attestation digest")
		return 2
	}
	artifacts, _, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	inspection, err := attestation.Inspect(artifacts, flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "inspect-attestation: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, inspection); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runInspectAttestationLink(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood inspect-attestation-link", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	custodyID := flags.String("id", "", "target custody record ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 || *root == "" || *custodyID == "" {
		fmt.Fprintln(stderr, "inspect-attestation-link requires --root, --id, and exactly one attestation digest")
		return 2
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	record, err := records.Get(*custodyID)
	if err != nil {
		fmt.Fprintf(stderr, "inspect-attestation-link: read custody record: %v\n", err)
		return 1
	}
	link, err := attestation.InspectLink(record, flags.Arg(0), artifacts)
	if err != nil {
		fmt.Fprintf(stderr, "inspect-attestation-link: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, link); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runInspectRedactionProvenance(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood inspect-redaction-provenance", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 || *root == "" {
		fmt.Fprintln(stderr, "inspect-redaction-provenance requires --root and exactly one provenance digest")
		return 2
	}
	artifacts, _, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	inspection, err := attestation.InspectRedactionProvenance(artifacts, flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "inspect-redaction-provenance: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, inspection); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runInspectRedactionProvenanceLink(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood inspect-redaction-provenance-link", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	sourceID := flags.String("source-id", "", "source custody record ID")
	eventID := flags.String("event-id", "", "redaction event ID")
	promotedID := flags.String("promoted-id", "", "promoted result custody record ID")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 || *root == "" || *sourceID == "" || *eventID == "" || *promotedID == "" {
		fmt.Fprintln(stderr, "inspect-redaction-provenance-link requires --root, --source-id, --event-id, --promoted-id, and exactly one provenance digest")
		return 2
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	source, err := custody.VerifyRecord(records, artifacts, *sourceID)
	if err != nil {
		fmt.Fprintf(stderr, "inspect-redaction-provenance-link: verify source custody record: %v\n", err)
		return 1
	}
	event, err := records.GetEvent(*sourceID, *eventID)
	if err != nil {
		fmt.Fprintf(stderr, "inspect-redaction-provenance-link: read redaction event: %v\n", err)
		return 1
	}
	promoted, err := custody.VerifyRecord(records, artifacts, *promotedID)
	if err != nil {
		fmt.Fprintf(stderr, "inspect-redaction-provenance-link: verify promoted custody record: %v\n", err)
		return 1
	}
	link, err := attestation.InspectRedactionProvenanceLink(source, event, promoted, flags.Arg(0), records, artifacts)
	if err != nil {
		fmt.Fprintf(stderr, "inspect-redaction-provenance-link: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, link); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runRecordDigest(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood record-digest", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 || *root == "" {
		fmt.Fprintln(stderr, "record-digest requires --root and exactly one custody ID")
		return 2
	}
	_, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	record, err := records.Get(flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "record-digest: %v\n", err)
		return 1
	}
	digest, err := custody.CanonicalDigest(record)
	if err != nil {
		fmt.Fprintf(stderr, "record-digest: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, digest)
	return 0
}

func runFindAttestation(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood find-attestation", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	targetDigest := flags.String("target-digest", "", "custody-record digest to match")
	keyID := flags.String("key-id", "", "attestation key ID to match")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *root == "" {
		fmt.Fprintln(stderr, "find-attestation requires --root and no positional arguments")
		return 2
	}
	artifacts, _, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	results, err := attestation.Find(artifacts, attestation.Query{TargetDigest: *targetDigest, KeyID: *keyID})
	if err != nil {
		fmt.Fprintf(stderr, "find-attestation: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, results); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runFindRedactionProvenance(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood find-redaction-provenance", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	sourceID := flags.String("source-id", "", "source custody record ID to match")
	eventID := flags.String("event-id", "", "redaction event ID to match")
	promotedID := flags.String("promoted-id", "", "promoted result custody record ID to match")
	keyID := flags.String("key-id", "", "provenance signing key ID to match")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *root == "" {
		fmt.Fprintln(stderr, "find-redaction-provenance requires --root and no positional arguments")
		return 2
	}
	artifacts, _, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	results, err := attestation.FindRedactionProvenance(artifacts, attestation.RedactionProvenanceQuery{
		SourceCustodyID:   *sourceID,
		EventID:           *eventID,
		PromotedCustodyID: *promotedID,
		KeyID:             *keyID,
	})
	if err != nil {
		fmt.Fprintf(stderr, "find-redaction-provenance: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, results); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runFindTrustedAttestation(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood find-trusted-attestation", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	targetDigest := flags.String("target-digest", "", "custody-record digest to match")
	keyID := flags.String("key-id", "", "attestation key ID to match")
	registryPath := flags.String("registry", "", "canonical attestation trust registry file")
	evaluatedAt := flags.String("at", "", "RFC3339 trust evaluation time; defaults to current UTC time")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *root == "" || *registryPath == "" {
		fmt.Fprintln(stderr, "find-trusted-attestation requires --root, --registry, and no positional arguments")
		return 2
	}
	when, err := parseTime(*evaluatedAt)
	if err != nil {
		fmt.Fprintf(stderr, "invalid --at: %v\n", err)
		return 2
	}
	if when.IsZero() {
		when = time.Now().UTC()
	}
	registry, err := readTrustRegistryFile(*registryPath)
	if err != nil {
		fmt.Fprintf(stderr, "find-trusted-attestation: %v\n", err)
		return 1
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	results, err := attestation.FindTrusted(records, artifacts, attestation.Query{TargetDigest: *targetDigest, KeyID: *keyID}, registry, when)
	if err != nil {
		fmt.Fprintf(stderr, "find-trusted-attestation: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, results); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runFindTrustedRedactionProvenance(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood find-trusted-redaction-provenance", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	sourceID := flags.String("source-id", "", "source custody record ID to match")
	eventID := flags.String("event-id", "", "redaction event ID to match")
	promotedID := flags.String("promoted-id", "", "promoted result custody record ID to match")
	keyID := flags.String("key-id", "", "provenance signing key ID to match")
	registryPath := flags.String("registry", "", "canonical attestation trust registry file")
	evaluatedAt := flags.String("at", "", "RFC3339 trust evaluation time; defaults to current UTC time")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *root == "" || *registryPath == "" {
		fmt.Fprintln(stderr, "find-trusted-redaction-provenance requires --root, --registry, and no positional arguments")
		return 2
	}
	when, err := parseTime(*evaluatedAt)
	if err != nil {
		fmt.Fprintf(stderr, "invalid --at: %v\n", err)
		return 2
	}
	if when.IsZero() {
		when = time.Now().UTC()
	}
	registry, err := readTrustRegistryFile(*registryPath)
	if err != nil {
		fmt.Fprintf(stderr, "find-trusted-redaction-provenance: %v\n", err)
		return 1
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	results, err := attestation.FindTrustedRedactionProvenance(records, records, artifacts, attestation.RedactionProvenanceQuery{
		SourceCustodyID:   *sourceID,
		EventID:           *eventID,
		PromotedCustodyID: *promotedID,
		KeyID:             *keyID,
	}, registry, when)
	if err != nil {
		fmt.Fprintf(stderr, "find-trusted-redaction-provenance: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, results); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runSignAttestation(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood sign-attestation", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	custodyID := flags.String("id", "", "target custody record ID")
	keyID := flags.String("key-id", "", "attestation signing key ID")
	privateKeyPath := flags.String("private-key", "", "base64 Ed25519 private-key file")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *root == "" || *custodyID == "" || *keyID == "" || *privateKeyPath == "" {
		fmt.Fprintln(stderr, "sign-attestation requires --root, --id, --key-id, --private-key, and no positional arguments")
		return 2
	}
	privateKey, err := readPrivateKeyFile(*privateKeyPath)
	if err != nil {
		fmt.Fprintf(stderr, "sign-attestation: %v\n", err)
		return 1
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	record, err := custody.VerifyRecord(records, artifacts, *custodyID)
	if err != nil {
		fmt.Fprintf(stderr, "sign-attestation: verify custody record: %v\n", err)
		return 1
	}
	envelope, err := attestation.Sign(record, *keyID, privateKey)
	if err != nil {
		fmt.Fprintf(stderr, "sign-attestation: %v\n", err)
		return 1
	}
	publication, err := attestation.PublishForRecord(record, envelope, artifacts)
	if err != nil {
		fmt.Fprintf(stderr, "sign-attestation: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, publication); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runSignHandlingEvent(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood sign-handling-event", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	custodyID := flags.String("id", "", "target custody record ID")
	eventID := flags.String("event-id", "", "target handling event ID")
	keyID := flags.String("key-id", "", "handling-event signing key ID")
	privateKeyPath := flags.String("private-key", "", "base64 Ed25519 private-key file")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *root == "" || *custodyID == "" || *eventID == "" || *keyID == "" || *privateKeyPath == "" {
		fmt.Fprintln(stderr, "sign-handling-event requires --root, --id, --event-id, --key-id, --private-key, and no positional arguments")
		return 2
	}
	privateKey, err := readPrivateKeyFile(*privateKeyPath)
	if err != nil {
		fmt.Fprintf(stderr, "sign-handling-event: %v\n", err)
		return 1
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	event, err := records.GetEvent(*custodyID, *eventID)
	if err != nil {
		fmt.Fprintf(stderr, "sign-handling-event: read handling event: %v\n", err)
		return 1
	}
	envelope, err := attestation.SignHandlingEvent(event, *keyID, privateKey)
	if err != nil {
		fmt.Fprintf(stderr, "sign-handling-event: %v\n", err)
		return 1
	}
	publication, err := attestation.PublishHandlingEvent(event, envelope, artifacts)
	if err != nil {
		fmt.Fprintf(stderr, "sign-handling-event: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, publication); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runSignRedactionProvenance(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood sign-redaction-provenance", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	sourceID := flags.String("source-id", "", "source custody record ID")
	eventID := flags.String("event-id", "", "redaction event ID")
	promotedID := flags.String("promoted-id", "", "promoted result custody record ID")
	keyID := flags.String("key-id", "", "redaction provenance signing key ID")
	privateKeyPath := flags.String("private-key", "", "base64 Ed25519 private-key file")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *root == "" || *sourceID == "" || *eventID == "" || *promotedID == "" || *keyID == "" || *privateKeyPath == "" {
		fmt.Fprintln(stderr, "sign-redaction-provenance requires --root, --source-id, --event-id, --promoted-id, --key-id, --private-key, and no positional arguments")
		return 2
	}
	privateKey, err := readPrivateKeyFile(*privateKeyPath)
	if err != nil {
		fmt.Fprintf(stderr, "sign-redaction-provenance: %v\n", err)
		return 1
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	source, err := custody.VerifyRecord(records, artifacts, *sourceID)
	if err != nil {
		fmt.Fprintf(stderr, "sign-redaction-provenance: verify source custody record: %v\n", err)
		return 1
	}
	event, err := records.GetEvent(*sourceID, *eventID)
	if err != nil {
		fmt.Fprintf(stderr, "sign-redaction-provenance: read redaction event: %v\n", err)
		return 1
	}
	promoted, err := custody.VerifyRecord(records, artifacts, *promotedID)
	if err != nil {
		fmt.Fprintf(stderr, "sign-redaction-provenance: verify promoted custody record: %v\n", err)
		return 1
	}
	envelope, err := attestation.SignRedactionProvenance(source, event, promoted, *keyID, privateKey)
	if err != nil {
		fmt.Fprintf(stderr, "sign-redaction-provenance: %v\n", err)
		return 1
	}
	publication, err := attestation.PublishRedactionProvenance(source, event, promoted, envelope, artifacts)
	if err != nil {
		fmt.Fprintf(stderr, "sign-redaction-provenance: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, publication); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runImportAttestation(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood import-attestation", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	custodyID := flags.String("id", "", "target custody record ID")
	expectedDigest := flags.String("expected-digest", "", "optional expected SHA-256 envelope digest")
	maxBytes := flags.Int64("max-bytes", 0, "maximum envelope size in bytes; 0 means unlimited")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 || *root == "" || *custodyID == "" {
		fmt.Fprintln(stderr, "import-attestation requires --root, --id, and exactly one envelope path")
		return 2
	}
	file, err := os.Open(flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "import-attestation: open envelope: %v\n", err)
		return 1
	}
	data, result, readErr := integrity.ReadAll(file, *maxBytes)
	closeErr := file.Close()
	if readErr != nil {
		fmt.Fprintf(stderr, "import-attestation: read envelope: %v\n", readErr)
		return 1
	}
	if closeErr != nil {
		fmt.Fprintf(stderr, "import-attestation: close envelope: %v\n", closeErr)
		return 1
	}
	if err := integrity.VerifyDigest(result.Digest, *expectedDigest); err != nil {
		fmt.Fprintf(stderr, "import-attestation: %v\n", err)
		return 1
	}
	envelope, err := attestation.UnmarshalCanonical(data)
	if err != nil {
		fmt.Fprintf(stderr, "import-attestation: %v\n", err)
		return 1
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	record, err := custody.VerifyRecord(records, artifacts, *custodyID)
	if err != nil {
		fmt.Fprintf(stderr, "import-attestation: verify custody record: %v\n", err)
		return 1
	}
	publication, err := attestation.PublishForRecord(record, envelope, artifacts)
	if err != nil {
		fmt.Fprintf(stderr, "import-attestation: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, publication); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runImportRedactionProvenance(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood import-redaction-provenance", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	sourceID := flags.String("source-id", "", "source custody record ID")
	eventID := flags.String("event-id", "", "redaction event ID")
	promotedID := flags.String("promoted-id", "", "promoted result custody record ID")
	expectedDigest := flags.String("expected-digest", "", "optional expected SHA-256 envelope digest")
	maxBytes := flags.Int64("max-bytes", 0, "maximum envelope size in bytes; 0 means unlimited")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 || *root == "" || *sourceID == "" || *eventID == "" || *promotedID == "" {
		fmt.Fprintln(stderr, "import-redaction-provenance requires --root, --source-id, --event-id, --promoted-id, and exactly one envelope path")
		return 2
	}
	file, err := os.Open(flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "import-redaction-provenance: open envelope: %v\n", err)
		return 1
	}
	data, result, readErr := integrity.ReadAll(file, *maxBytes)
	closeErr := file.Close()
	if readErr != nil {
		fmt.Fprintf(stderr, "import-redaction-provenance: read envelope: %v\n", readErr)
		return 1
	}
	if closeErr != nil {
		fmt.Fprintf(stderr, "import-redaction-provenance: close envelope: %v\n", closeErr)
		return 1
	}
	if err := integrity.VerifyDigest(result.Digest, *expectedDigest); err != nil {
		fmt.Fprintf(stderr, "import-redaction-provenance: %v\n", err)
		return 1
	}
	envelope, err := attestation.UnmarshalCanonicalRedactionProvenance(data)
	if err != nil {
		fmt.Fprintf(stderr, "import-redaction-provenance: %v\n", err)
		return 1
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	source, err := custody.VerifyRecord(records, artifacts, *sourceID)
	if err != nil {
		fmt.Fprintf(stderr, "import-redaction-provenance: verify source custody record: %v\n", err)
		return 1
	}
	event, err := records.GetEvent(*sourceID, *eventID)
	if err != nil {
		fmt.Fprintf(stderr, "import-redaction-provenance: read redaction event: %v\n", err)
		return 1
	}
	promoted, err := custody.VerifyRecord(records, artifacts, *promotedID)
	if err != nil {
		fmt.Fprintf(stderr, "import-redaction-provenance: verify promoted custody record: %v\n", err)
		return 1
	}
	publication, err := attestation.PublishRedactionProvenance(source, event, promoted, envelope, artifacts)
	if err != nil {
		fmt.Fprintf(stderr, "import-redaction-provenance: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, publication); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runVerifyAttestation(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood verify-attestation", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	custodyID := flags.String("id", "", "target custody record ID")
	publicKeyPath := flags.String("public-key", "", "base64 Ed25519 public-key file")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 || *root == "" || *custodyID == "" || *publicKeyPath == "" {
		fmt.Fprintln(stderr, "verify-attestation requires --root, --id, --public-key, and exactly one attestation digest")
		return 2
	}
	publicKey, err := readPublicKeyFile(*publicKeyPath)
	if err != nil {
		fmt.Fprintf(stderr, "verify-attestation: %v\n", err)
		return 1
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	record, err := custody.VerifyRecord(records, artifacts, *custodyID)
	if err != nil {
		fmt.Fprintf(stderr, "verify-attestation: verify custody record: %v\n", err)
		return 1
	}
	envelope, err := attestation.Load(artifacts, flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "verify-attestation: %v\n", err)
		return 1
	}
	if err := attestation.Verify(record, envelope, publicKey); err != nil {
		fmt.Fprintf(stderr, "verify-attestation: %v\n", err)
		return 1
	}
	receipt := attestation.VerificationReceipt{
		CustodyID:         record.CustodyID,
		AttestationDigest: flags.Arg(0),
		TargetDigest:      envelope.Target.Digest,
		KeyID:             envelope.KeyID,
		Algorithm:         envelope.Algorithm,
		Verified:          true,
	}
	if err := writeJSON(stdout, receipt); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runVerifyAttestationTrusted(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood verify-attestation-trusted", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	custodyID := flags.String("id", "", "target custody record ID")
	registryPath := flags.String("registry", "", "canonical attestation trust registry file")
	evaluatedAt := flags.String("at", "", "RFC3339 trust evaluation time; defaults to current UTC time")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 || *root == "" || *custodyID == "" || *registryPath == "" {
		fmt.Fprintln(stderr, "verify-attestation-trusted requires --root, --id, --registry, and exactly one attestation digest")
		return 2
	}
	when, err := parseTime(*evaluatedAt)
	if err != nil {
		fmt.Fprintf(stderr, "invalid --at: %v\n", err)
		return 2
	}
	if when.IsZero() {
		when = time.Now().UTC()
	}
	registry, err := readTrustRegistryFile(*registryPath)
	if err != nil {
		fmt.Fprintf(stderr, "verify-attestation-trusted: %v\n", err)
		return 1
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	record, err := custody.VerifyRecord(records, artifacts, *custodyID)
	if err != nil {
		fmt.Fprintf(stderr, "verify-attestation-trusted: verify custody record: %v\n", err)
		return 1
	}
	receipt, err := attestation.VerifyPublishedWithRegistryReceipt(record, flags.Arg(0), artifacts, registry, when)
	if err != nil {
		fmt.Fprintf(stderr, "verify-attestation-trusted: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, receipt); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runVerifyHandlingEvent(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood verify-handling-event", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	custodyID := flags.String("id", "", "target custody record ID")
	eventID := flags.String("event-id", "", "target handling event ID")
	publicKeyPath := flags.String("public-key", "", "base64 Ed25519 public-key file")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 || *root == "" || *custodyID == "" || *eventID == "" || *publicKeyPath == "" {
		fmt.Fprintln(stderr, "verify-handling-event requires --root, --id, --event-id, --public-key, and exactly one attestation digest")
		return 2
	}
	publicKey, err := readPublicKeyFile(*publicKeyPath)
	if err != nil {
		fmt.Fprintf(stderr, "verify-handling-event: %v\n", err)
		return 1
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	event, err := records.GetEvent(*custodyID, *eventID)
	if err != nil {
		fmt.Fprintf(stderr, "verify-handling-event: read handling event: %v\n", err)
		return 1
	}
	receipt, err := attestation.VerifyPublishedHandlingEventReceipt(event, flags.Arg(0), artifacts, publicKey)
	if err != nil {
		fmt.Fprintf(stderr, "verify-handling-event: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, receipt); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runVerifyHandlingEventTrusted(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood verify-handling-event-trusted", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	custodyID := flags.String("id", "", "target custody record ID")
	eventID := flags.String("event-id", "", "target handling event ID")
	registryPath := flags.String("registry", "", "canonical attestation trust registry file")
	evaluatedAt := flags.String("at", "", "RFC3339 trust evaluation time; defaults to current UTC time")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 || *root == "" || *custodyID == "" || *eventID == "" || *registryPath == "" {
		fmt.Fprintln(stderr, "verify-handling-event-trusted requires --root, --id, --event-id, --registry, and exactly one attestation digest")
		return 2
	}
	when, err := parseTime(*evaluatedAt)
	if err != nil {
		fmt.Fprintf(stderr, "invalid --at: %v\n", err)
		return 2
	}
	if when.IsZero() {
		when = time.Now().UTC()
	}
	registry, err := readTrustRegistryFile(*registryPath)
	if err != nil {
		fmt.Fprintf(stderr, "verify-handling-event-trusted: %v\n", err)
		return 1
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	event, err := records.GetEvent(*custodyID, *eventID)
	if err != nil {
		fmt.Fprintf(stderr, "verify-handling-event-trusted: read handling event: %v\n", err)
		return 1
	}
	receipt, err := attestation.VerifyPublishedHandlingEventWithRegistryReceipt(event, flags.Arg(0), artifacts, registry, when)
	if err != nil {
		fmt.Fprintf(stderr, "verify-handling-event-trusted: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, receipt); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runVerifyHandlingEventAuthorized(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood verify-handling-event-authorized", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	custodyID := flags.String("id", "", "target custody record ID")
	eventID := flags.String("event-id", "", "target handling event ID")
	registryPath := flags.String("registry", "", "canonical attestation trust registry file")
	policyPath := flags.String("policy", "", "canonical handling authorization policy file")
	evaluatedAt := flags.String("at", "", "RFC3339 trust and policy evaluation time; defaults to current UTC time")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 || *root == "" || *custodyID == "" || *eventID == "" || *registryPath == "" || *policyPath == "" {
		fmt.Fprintln(stderr, "verify-handling-event-authorized requires --root, --id, --event-id, --registry, --policy, and exactly one attestation digest")
		return 2
	}
	when, err := parseTime(*evaluatedAt)
	if err != nil {
		fmt.Fprintf(stderr, "invalid --at: %v\n", err)
		return 2
	}
	if when.IsZero() {
		when = time.Now().UTC()
	}
	registry, err := readTrustRegistryFile(*registryPath)
	if err != nil {
		fmt.Fprintf(stderr, "verify-handling-event-authorized: %v\n", err)
		return 1
	}
	policy, err := readHandlingAuthorizationPolicyFile(*policyPath)
	if err != nil {
		fmt.Fprintf(stderr, "verify-handling-event-authorized: %v\n", err)
		return 1
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	event, err := records.GetEvent(*custodyID, *eventID)
	if err != nil {
		fmt.Fprintf(stderr, "verify-handling-event-authorized: read handling event: %v\n", err)
		return 1
	}
	receipt, err := attestation.VerifyPublishedHandlingEventWithRegistryAndPolicyReceipt(event, flags.Arg(0), artifacts, registry, policy, when)
	if err != nil {
		fmt.Fprintf(stderr, "verify-handling-event-authorized: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, receipt); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runVerifyRedactionProvenance(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood verify-redaction-provenance", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	sourceID := flags.String("source-id", "", "source custody record ID")
	eventID := flags.String("event-id", "", "redaction event ID")
	promotedID := flags.String("promoted-id", "", "promoted result custody record ID")
	publicKeyPath := flags.String("public-key", "", "base64 Ed25519 public-key file")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 || *root == "" || *sourceID == "" || *eventID == "" || *promotedID == "" || *publicKeyPath == "" {
		fmt.Fprintln(stderr, "verify-redaction-provenance requires --root, --source-id, --event-id, --promoted-id, --public-key, and exactly one provenance digest")
		return 2
	}
	publicKey, err := readPublicKeyFile(*publicKeyPath)
	if err != nil {
		fmt.Fprintf(stderr, "verify-redaction-provenance: %v\n", err)
		return 1
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	source, err := custody.VerifyRecord(records, artifacts, *sourceID)
	if err != nil {
		fmt.Fprintf(stderr, "verify-redaction-provenance: verify source custody record: %v\n", err)
		return 1
	}
	event, err := records.GetEvent(*sourceID, *eventID)
	if err != nil {
		fmt.Fprintf(stderr, "verify-redaction-provenance: read redaction event: %v\n", err)
		return 1
	}
	promoted, err := custody.VerifyRecord(records, artifacts, *promotedID)
	if err != nil {
		fmt.Fprintf(stderr, "verify-redaction-provenance: verify promoted custody record: %v\n", err)
		return 1
	}
	receipt, err := attestation.VerifyPublishedRedactionProvenanceReceipt(source, event, promoted, flags.Arg(0), records, artifacts, publicKey)
	if err != nil {
		fmt.Fprintf(stderr, "verify-redaction-provenance: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, receipt); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runVerifyRedactionProvenanceTrusted(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood verify-redaction-provenance-trusted", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	sourceID := flags.String("source-id", "", "source custody record ID")
	eventID := flags.String("event-id", "", "redaction event ID")
	promotedID := flags.String("promoted-id", "", "promoted result custody record ID")
	registryPath := flags.String("registry", "", "canonical attestation trust registry file")
	evaluatedAt := flags.String("at", "", "RFC3339 trust evaluation time; defaults to current UTC time")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 || *root == "" || *sourceID == "" || *eventID == "" || *promotedID == "" || *registryPath == "" {
		fmt.Fprintln(stderr, "verify-redaction-provenance-trusted requires --root, --source-id, --event-id, --promoted-id, --registry, and exactly one provenance digest")
		return 2
	}
	when, err := parseTime(*evaluatedAt)
	if err != nil {
		fmt.Fprintf(stderr, "invalid --at: %v\n", err)
		return 2
	}
	if when.IsZero() {
		when = time.Now().UTC()
	}
	registry, err := readTrustRegistryFile(*registryPath)
	if err != nil {
		fmt.Fprintf(stderr, "verify-redaction-provenance-trusted: %v\n", err)
		return 1
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	source, err := custody.VerifyRecord(records, artifacts, *sourceID)
	if err != nil {
		fmt.Fprintf(stderr, "verify-redaction-provenance-trusted: verify source custody record: %v\n", err)
		return 1
	}
	event, err := records.GetEvent(*sourceID, *eventID)
	if err != nil {
		fmt.Fprintf(stderr, "verify-redaction-provenance-trusted: read redaction event: %v\n", err)
		return 1
	}
	promoted, err := custody.VerifyRecord(records, artifacts, *promotedID)
	if err != nil {
		fmt.Fprintf(stderr, "verify-redaction-provenance-trusted: verify promoted custody record: %v\n", err)
		return 1
	}
	receipt, err := attestation.VerifyPublishedRedactionProvenanceWithRegistryReceipt(source, event, promoted, flags.Arg(0), records, artifacts, registry, when)
	if err != nil {
		fmt.Fprintf(stderr, "verify-redaction-provenance-trusted: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, receipt); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runVerify(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood verify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	custodyID := flags.String("id", "", "verify one custody record instead of a raw digest")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *root == "" || (*custodyID != "" && flags.NArg() != 0) || (*custodyID == "" && flags.NArg() != 1) {
		fmt.Fprintln(stderr, "verify requires --root and either one digest or --id <custody-id>")
		return 2
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	if *custodyID != "" {
		record, err := custody.VerifyRecord(records, artifacts, *custodyID)
		if err != nil {
			fmt.Fprintf(stderr, "verify: %v\n", err)
			return 1
		}
		if err := writeJSON(stdout, record); err != nil {
			fmt.Fprintf(stderr, "write result: %v\n", err)
			return 1
		}
		return 0
	}
	if err := artifacts.Verify(flags.Arg(0)); err != nil {
		fmt.Fprintf(stderr, "verify: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, flags.Arg(0))
	return 0
}

func runFind(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood find", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	custodyID := flags.String("id", "", "custody record ID")
	digest := flags.String("digest", "", "artifact digest")
	runID := flags.String("run-id", "", "producer run ID")
	sourceURI := flags.String("source-uri", "", "remote source URI")
	sourceVersion := flags.String("source-version", "", "remote source version or object version")
	producer := flags.String("producer", "", "producing tool")
	kind := flags.String("kind", "", "producer artifact kind")
	status := flags.String("status", "", "custody status")
	mediaType := flags.String("media-type", "", "artifact media type")
	logicalName := flags.String("name", "", "artifact logical name")
	retentionClass := flags.String("retention-class", "", "retention class")
	parentDigest := flags.String("parent-digest", "", "parent artifact digest")
	parentRelation := flags.String("parent-relation", "", "parent lineage relation")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *root == "" {
		fmt.Fprintln(stderr, "find requires --root and no positional arguments")
		return 2
	}
	if *status != "" && *status != string(custody.Accepted) && *status != string(custody.Quarantined) && *status != string(custody.Rejected) {
		fmt.Fprintf(stderr, "invalid status %q\n", *status)
		return 2
	}
	if *parentRelation != "" {
		switch custody.Relation(*parentRelation) {
		case custody.References, custody.DerivedFrom, custody.Contains, custody.Verifies:
		default:
			fmt.Fprintf(stderr, "invalid parent relation %q\n", *parentRelation)
			return 2
		}
	}
	if *parentDigest != "" {
		if err := artifact.ValidateDigest(*parentDigest); err != nil {
			fmt.Fprintf(stderr, "invalid parent digest: %v\n", err)
			return 2
		}
	}
	_, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	cat, err := catalog.New(records)
	if err != nil {
		fmt.Fprintf(stderr, "create catalog: %v\n", err)
		return 1
	}
	results, err := cat.Find(catalog.Query{
		CustodyID:      *custodyID,
		Digest:         *digest,
		RunID:          *runID,
		SourceURI:      *sourceURI,
		SourceVersion:  *sourceVersion,
		ProducerTool:   *producer,
		ProducerKind:   *kind,
		Status:         custody.Status(*status),
		MediaType:      *mediaType,
		LogicalName:    *logicalName,
		RetentionClass: *retentionClass,
		ParentDigest:   *parentDigest,
		ParentRelation: custody.Relation(*parentRelation),
	})
	if err != nil {
		fmt.Fprintf(stderr, "find: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, results); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runRecover(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood recover", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 || *root == "" {
		fmt.Fprintln(stderr, "recover requires --root and exactly one pending record path")
		return 2
	}

	file, err := os.Open(flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "open pending record: %v\n", err)
		return 1
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var pending custody.Record
	if err := decoder.Decode(&pending); err != nil {
		fmt.Fprintf(stderr, "decode pending record: %v\n", err)
		return 1
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			fmt.Fprintln(stderr, "decode pending record: multiple JSON values")
		} else {
			fmt.Fprintf(stderr, "decode pending record: %v\n", err)
		}
		return 1
	}

	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	ingestor, err := custody.NewIngestor(artifacts, records)
	if err != nil {
		fmt.Fprintf(stderr, "create ingestor: %v\n", err)
		return 1
	}
	recovered, err := ingestor.Recover(pending)
	if err != nil {
		fmt.Fprintf(stderr, "recover: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, recovered); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func runReconcile(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood reconcile", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	orphanGrace := flags.Duration("orphan-grace", 0, "classify valid orphans at least this old as cleanup candidates; no deletion")
	asOf := flags.String("as-of", "", "RFC3339 time used for orphan age classification; defaults to current UTC time")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *root == "" {
		fmt.Fprintln(stderr, "reconcile requires --root and no positional arguments")
		return 2
	}
	if *orphanGrace < 0 {
		fmt.Fprintln(stderr, "reconcile: --orphan-grace cannot be negative")
		return 2
	}
	when, err := parseTime(*asOf)
	if err != nil {
		fmt.Fprintf(stderr, "invalid --as-of: %v\n", err)
		return 2
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	report, err := custody.ReconcileWithOptions(artifacts, records, custody.ReconcileOptions{
		OrphanGrace:        *orphanGrace,
		Now:                when,
		DetachedMediaTypes: []string{attestation.MediaType, attestation.HandlingEventMediaType, attestation.RedactionProvenanceMediaType},
	})
	if err != nil {
		fmt.Fprintf(stderr, "reconcile: %v\n", err)
		return 1
	}
	if err := writeJSON(stdout, report); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 1
	}
	return 0
}

func openStores(root string) (*store.Filesystem, *custody.Filesystem, error) {
	artifacts, err := store.NewFilesystem(root)
	if err != nil {
		return nil, nil, err
	}
	records, err := custody.NewFilesystem(root)
	if err != nil {
		return nil, nil, err
	}
	return artifacts, records, nil
}

func readPublicKeyFile(path string) (ed25519.PublicKey, error) {
	decoded, err := readBase64KeyFile(path, ed25519.PublicKeySize, "public key", false)
	if err != nil {
		return nil, err
	}
	return ed25519.PublicKey(decoded), nil
}

func readPrivateKeyFile(path string) (ed25519.PrivateKey, error) {
	decoded, err := readBase64KeyFile(path, ed25519.PrivateKeySize, "private key", true)
	if err != nil {
		return nil, err
	}
	return ed25519.PrivateKey(decoded), nil
}

func readTrustRegistryFile(path string) (attestation.TrustRegistry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return attestation.TrustRegistry{}, fmt.Errorf("stat trust registry: %w", err)
	}
	if !info.Mode().IsRegular() {
		return attestation.TrustRegistry{}, fmt.Errorf("trust registry must be a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return attestation.TrustRegistry{}, fmt.Errorf("read trust registry: %w", err)
	}
	registry, err := attestation.UnmarshalCanonicalTrustRegistry(data)
	if err != nil {
		return attestation.TrustRegistry{}, err
	}
	return registry, nil
}

func readHandlingAuthorizationPolicyFile(path string) (attestation.HandlingAuthorizationPolicy, error) {
	info, err := os.Stat(path)
	if err != nil {
		return attestation.HandlingAuthorizationPolicy{}, fmt.Errorf("stat handling authorization policy: %w", err)
	}
	if !info.Mode().IsRegular() {
		return attestation.HandlingAuthorizationPolicy{}, fmt.Errorf("handling authorization policy must be a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return attestation.HandlingAuthorizationPolicy{}, fmt.Errorf("read handling authorization policy: %w", err)
	}
	policy, err := attestation.UnmarshalCanonicalHandlingAuthorizationPolicy(data)
	if err != nil {
		return attestation.HandlingAuthorizationPolicy{}, err
	}
	return policy, nil
}

func readBase64KeyFile(path string, expectedSize int, label string, restrictPermissions bool) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", label, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s must be a regular file", label)
	}
	if restrictPermissions && info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s file permissions are too open; require 0600 or stricter", label)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", label, err)
	}
	encoded := string(data)
	if strings.HasSuffix(encoded, "\n") {
		encoded = strings.TrimSuffix(encoded, "\n")
		if strings.HasSuffix(encoded, "\r") {
			encoded = strings.TrimSuffix(encoded, "\r")
		}
	}
	if encoded == "" || strings.ContainsAny(encoded, " \t\r\n") {
		return nil, fmt.Errorf("%s must be base64 with at most one trailing newline", label)
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", label, err)
	}
	if len(decoded) != expectedSize {
		return nil, fmt.Errorf("%s has size %d, want %d", label, len(decoded), expectedSize)
	}
	return decoded, nil
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func savePendingRecord(path string, intakeErr error) (bool, error) {
	if path == "" {
		return false, nil
	}
	var pending *custody.IntakeError
	if !errors.As(intakeErr, &pending) {
		return false, nil
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return false, err
	}
	removeFile := true
	defer func() {
		_ = file.Close()
		if removeFile {
			_ = os.Remove(path)
		}
	}()
	if err := writeJSON(file, pending.Record); err != nil {
		return false, err
	}
	if err := file.Close(); err != nil {
		return false, err
	}
	removeFile = false
	return true, nil
}

func parseTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, value)
}

type lineageFlags []custody.Lineage

func (flags *lineageFlags) String() string {
	values := make([]string, 0, len(*flags))
	for _, parent := range *flags {
		values = append(values, string(parent.Relation)+"="+parent.Digest)
	}
	return strings.Join(values, ",")
}

func (flags *lineageFlags) Set(value string) error {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return errors.New("parent must be relation=digest")
	}
	parent := custody.Lineage{Relation: custody.Relation(parts[0]), Digest: parts[1]}
	switch parent.Relation {
	case custody.References, custody.DerivedFrom, custody.Contains, custody.Verifies:
	default:
		return fmt.Errorf("unsupported lineage relation %q", parent.Relation)
	}
	if err := artifact.ValidateDigest(parent.Digest); err != nil {
		return err
	}
	*flags = append(*flags, parent)
	return nil
}

func usage(writer io.Writer) {
	fmt.Fprintln(writer, "usage: lockwood <command> [options]")
	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "commands:")
	fmt.Fprintln(writer, "  put [options] <path|-> store bytes and create accepted custody")
	fmt.Fprintln(writer, "  import-sorna [options] <dir> import a verified Sorna bundle")
	fmt.Fprintln(writer, "  import-ci-result [options] <path> import a validated CI result")
	fmt.Fprintln(writer, "  get <digest>       retrieve verified bytes")
	fmt.Fprintln(writer, "  inspect <id>       print one custody record")
	fmt.Fprintln(writer, "  inspect-attestation inspect a published detached attestation")
	fmt.Fprintln(writer, "  inspect-attestation-link show its typed custody-record link")
	fmt.Fprintln(writer, "  inspect-redaction-provenance inspect detached redaction provenance")
	fmt.Fprintln(writer, "  inspect-redaction-provenance-link show its typed source-event-result link")
	fmt.Fprintln(writer, "  lineage-status     report reachable lineage resolution")
	fmt.Fprintln(writer, "  append-event       append an immutable handling event to a custody record")
	fmt.Fprintln(writer, "  list-events        list immutable handling events for a custody record")
	fmt.Fprintln(writer, "  handling-status    project recorded handling state without enforcing policy")
	fmt.Fprintln(writer, "  redaction-status   trace a redaction event and its custody promotion")
	fmt.Fprintln(writer, "  check-handling-guard report legal-hold blocking for redact/delete")
	fmt.Fprintln(writer, "  register-redaction register a verified caller-produced redaction derivative")
	fmt.Fprintln(writer, "  promote-redaction create custody for a registered redaction result")
	fmt.Fprintln(writer, "  sign-redaction-provenance sign and publish source-event-result provenance")
	fmt.Fprintln(writer, "  record-digest <id> print the canonical custody-record digest")
	fmt.Fprintln(writer, "  find-attestation   find published attestations by target or key ID")
	fmt.Fprintln(writer, "  find-redaction-provenance find published source-event-result provenance")
	fmt.Fprintln(writer, "  find-trusted-attestation find and verify attestations through a trust registry")
	fmt.Fprintln(writer, "  find-trusted-redaction-provenance find and verify provenance through a trust registry")
	fmt.Fprintln(writer, "  sign-attestation   sign and publish a detached attestation")
	fmt.Fprintln(writer, "  sign-handling-event sign and publish a detached handling-event signature")
	fmt.Fprintln(writer, "  import-attestation import and publish a detached attestation")
	fmt.Fprintln(writer, "  import-redaction-provenance import and publish detached source-event-result provenance")
	fmt.Fprintln(writer, "  verify-attestation verify a published attestation with an explicit public key")
	fmt.Fprintln(writer, "  verify-attestation-trusted verify with an explicit trust registry")
	fmt.Fprintln(writer, "  verify-handling-event verify a handling-event signature with an explicit public key")
	fmt.Fprintln(writer, "  verify-handling-event-trusted verify a handling-event signature through a trust registry")
	fmt.Fprintln(writer, "  verify-handling-event-authorized verify a handling event through trust and action policy")
	fmt.Fprintln(writer, "  verify-redaction-provenance verify signed source-event-result provenance")
	fmt.Fprintln(writer, "  verify-redaction-provenance-trusted verify provenance through a trust registry")
	fmt.Fprintln(writer, "  verify <digest>    verify stored bytes")
	fmt.Fprintln(writer, "  verify --id <id>   verify a custody record and its blob")
	fmt.Fprintln(writer, "  find               query custody records")
	fmt.Fprintln(writer, "  recover            verify an existing blob and append a pending custody record")
	fmt.Fprintln(writer, "  reconcile          report orphans, dangling records, and corrupt blobs")
}
