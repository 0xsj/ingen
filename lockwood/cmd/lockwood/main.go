package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ingen/lockwood/internal/adapters/sorna"
	"ingen/lockwood/internal/artifact"
	"ingen/lockwood/internal/catalog"
	"ingen/lockwood/internal/custody"
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
	case "get":
		return runGet(args[1:], stdout, stderr)
	case "inspect":
		return runInspect(args[1:], stdout, stderr)
	case "verify":
		return runVerify(args[1:], stdout, stderr)
	case "find":
		return runFind(args[1:], stdout, stderr)
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
		ReceivedAt:     parsedReceivedAt,
		Producer:       custody.Producer{Tool: *producer, Kind: *kind},
		Source:         custody.Source{RunID: *runID, Path: *sourcePath, URI: *sourceURI, Version: *sourceVersion},
		Parents:        []custody.Lineage(parents),
		Handling:       custody.Handling{Redaction: *redaction, RetentionClass: *retentionClass},
	})
	if err != nil {
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
		ReceivedAt:     parsedReceivedAt,
		RetentionClass: *retentionClass,
		Redaction:      *redaction,
	})
	if err != nil {
		fmt.Fprintf(stderr, "import-sorna: %v\n", err)
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

func runReconcile(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("lockwood reconcile", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", "", "Lockwood data root")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *root == "" {
		fmt.Fprintln(stderr, "reconcile requires --root and no positional arguments")
		return 2
	}
	artifacts, records, err := openStores(*root)
	if err != nil {
		fmt.Fprintf(stderr, "open Lockwood root: %v\n", err)
		return 1
	}
	report, err := custody.Reconcile(artifacts, records)
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

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
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
	fmt.Fprintln(writer, "  get <digest>       retrieve verified bytes")
	fmt.Fprintln(writer, "  inspect <id>       print one custody record")
	fmt.Fprintln(writer, "  verify <digest>    verify stored bytes")
	fmt.Fprintln(writer, "  verify --id <id>   verify a custody record and its blob")
	fmt.Fprintln(writer, "  find               query custody records")
	fmt.Fprintln(writer, "  reconcile          report orphans, dangling records, and corrupt blobs")
}
