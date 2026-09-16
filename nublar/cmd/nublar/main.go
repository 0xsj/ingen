package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ingen/core/ciresult"
	"ingen/nublar/internal/aggregate"
	nublardelivery "ingen/nublar/internal/delivery"
	nublarwebhook "ingen/nublar/internal/delivery/webhook"
	nublaroutput "ingen/nublar/internal/output"
	nublarrun "ingen/nublar/internal/run"
	nublarstore "ingen/nublar/internal/storage/filesystem"
	"ingen/nublar/internal/workflow"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	if args[0] == "workflow" {
		return workflowCommand(args[1:])
	}
	if args[0] == "run" {
		return runCommand(args[1:])
	}
	if args[0] != "aggregate" {
		usage()
		return 2
	}
	flags := flag.NewFlagSet("aggregate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	output := flags.String("output", "", "path for the Nublar aggregate; stdout when empty")
	workflowPath := flags.String("workflow", "", "Nublar workflow file declaring the result inputs")
	root := flags.String("root", ".", "workspace root for paths in --workflow")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if *workflowPath != "" && len(flags.Args()) > 0 {
		fmt.Fprintln(os.Stderr, "--workflow cannot be combined with positional CI results")
		return 2
	}
	if *workflowPath == "" && len(flags.Args()) == 0 {
		usage()
		return 2
	}
	var report aggregate.Report
	var err error
	if *workflowPath != "" {
		report, err = aggregate.ComposeWorkflowFile(*workflowPath, *root)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	} else {
		report, err = aggregate.ComposeFiles(flags.Args())
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}
	if *output == "" {
		if err := aggregate.WriteJSON(os.Stdout, report); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	} else if err := aggregate.SaveFile(*output, report); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return report.ExitCode
}

func workflowCommand(args []string) int {
	if len(args) != 2 || args[0] != "validate" {
		usage()
		return 2
	}
	if _, err := workflow.LoadFile(args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("valid:", filepath.Clean(args[1]))
	return 0
}

func runCommand(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "collect":
		return collectCommand(args[1:])
	case "show":
		return showCommand(args[1:])
	case "list":
		return listCommand(args[1:])
	case "decision":
		return decisionCommand(args[1:])
	case "deliver":
		return deliverCommand(args[1:])
	default:
		usage()
		return 2
	}
}

func collectCommand(args []string) int {
	flags := flag.NewFlagSet("run collect", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	workflowPath := flags.String("workflow", "", "Nublar workflow file")
	root := flags.String("root", ".", "workspace root for workflow result paths")
	output := flags.String("output", "", "path for the Nublar run; stdout when empty")
	storeRoot := flags.String("store", "", "filesystem store root; no durable save when empty")
	runID := flags.String("run-id", "", "opaque Nublar run ID; generated when empty")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *workflowPath == "" || len(flags.Args()) > 0 {
		fmt.Fprintln(os.Stderr, "run collect requires --workflow and does not accept positional arguments")
		return 2
	}
	if *runID == "" {
		generated, err := nublarrun.NewID()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		*runID = generated
	}
	report, err := nublarrun.CollectWorkflowFile(*workflowPath, *root, *runID)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if *storeRoot != "" {
		store, storeErr := nublarstore.New(*storeRoot)
		if storeErr != nil {
			fmt.Fprintln(os.Stderr, storeErr)
			return 2
		}
		if storeErr := store.Save(report); storeErr != nil {
			fmt.Fprintln(os.Stderr, storeErr)
			return 2
		}
	}
	if *output == "" {
		if err := nublarrun.WriteJSON(os.Stdout, report); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	} else if err := nublarrun.SaveFile(*output, report); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return report.ExitCode
}

func showCommand(args []string) int {
	flags := flag.NewFlagSet("run show", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRoot := flags.String("store", "", "filesystem store root")
	runID := flags.String("run-id", "", "Nublar run ID")
	output := flags.String("output", "", "path for the Nublar run; stdout when empty")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *storeRoot == "" || *runID == "" || len(flags.Args()) > 0 {
		fmt.Fprintln(os.Stderr, "run show requires --store and --run-id and does not accept positional arguments")
		return 2
	}
	store, err := nublarstore.New(*storeRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	record, err := store.Load(*runID)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if *output == "" {
		if err := nublarrun.WriteJSON(os.Stdout, record); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	} else if err := nublarrun.SaveFile(*output, record); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return record.ExitCode
}

func listCommand(args []string) int {
	flags := flag.NewFlagSet("run list", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRoot := flags.String("store", "", "filesystem store root")
	status := flags.String("status", "", "optional run status filter: passed, failed, or error")
	workflowID := flags.String("workflow", "", "optional workflow ID filter")
	output := flags.String("output", "", "path for the JSON run list; stdout when empty")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *storeRoot == "" || len(flags.Args()) > 0 {
		fmt.Fprintln(os.Stderr, "run list requires --store and does not accept positional arguments")
		return 2
	}
	store, err := nublarstore.New(*storeRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	records, err := store.List()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	records, err = filterRuns(records, *status, *workflowID)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if err := writeRunList(*output, records); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return 0
}

func filterRuns(records []nublarrun.Run, status, workflowID string) ([]nublarrun.Run, error) {
	status = strings.TrimSpace(status)
	workflowID = strings.TrimSpace(workflowID)
	if status != "" {
		if _, err := ciresult.ExitCodeForStatus(status); err != nil {
			return nil, fmt.Errorf("Nublar run list status filter: %w", err)
		}
	}
	filtered := make([]nublarrun.Run, 0, len(records))
	for _, record := range records {
		if status != "" && record.Status != status {
			continue
		}
		if workflowID != "" && record.Workflow.ID != workflowID {
			continue
		}
		filtered = append(filtered, record)
	}
	return filtered, nil
}

func decisionCommand(args []string) int {
	flags := flag.NewFlagSet("run decision", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRoot := flags.String("store", "", "filesystem store root")
	runID := flags.String("run-id", "", "Nublar run ID")
	output := flags.String("output", "", "path for the provider-neutral decision; stdout when empty")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *storeRoot == "" || *runID == "" || len(flags.Args()) > 0 {
		fmt.Fprintln(os.Stderr, "run decision requires --store and --run-id and does not accept positional arguments")
		return 2
	}
	store, err := nublarstore.New(*storeRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	record, err := store.Load(*runID)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	decision, err := nublardelivery.Project(record)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if *output == "" {
		if err := nublardelivery.WriteJSON(os.Stdout, decision); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	} else if err := saveDecision(*output, decision); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return 0
}

type webhookPublisherFactory func(endpoint string, secret []byte) (nublardelivery.Publisher, error)

func deliverCommand(args []string) int {
	return deliverCommandWithFactory(args, func(endpoint string, secret []byte) (nublardelivery.Publisher, error) {
		return nublarwebhook.NewWithSecret(endpoint, nil, secret)
	})
}

func deliverCommandWithFactory(args []string, newPublisher webhookPublisherFactory) int {
	flags := flag.NewFlagSet("run deliver", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	storeRoot := flags.String("store", "", "filesystem store root")
	runID := flags.String("run-id", "", "Nublar run ID")
	endpoint := flags.String("webhook", "", "HTTP(S) webhook endpoint")
	timeout := flags.Duration("timeout", 30*time.Second, "maximum time for one webhook delivery")
	secretEnv := flags.String("secret-env", "", "environment variable containing optional webhook HMAC secret")
	receiptPath := flags.String("receipt", "", "path for the delivery receipt; no receipt file when empty")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *storeRoot == "" || *runID == "" || *endpoint == "" || *timeout <= 0 || len(flags.Args()) > 0 {
		fmt.Fprintln(os.Stderr, "run deliver requires --store, --run-id, --webhook, and a positive --timeout")
		return 2
	}
	store, err := nublarstore.New(*storeRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	record, err := store.Load(*runID)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	decision, err := nublardelivery.Project(record)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	var secret []byte
	if *secretEnv != "" {
		value, ok := os.LookupEnv(*secretEnv)
		if !ok || value == "" {
			fmt.Fprintf(os.Stderr, "webhook secret environment variable %q is missing or empty\n", *secretEnv)
			return 2
		}
		secret = []byte(value)
	}
	publisher, err := newPublisher(*endpoint, secret)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	receipt, publishErr := publisher.Publish(ctx, decision)
	if *receiptPath != "" {
		if err := saveReceipt(*receiptPath, receipt); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}
	if publishErr != nil {
		fmt.Fprintln(os.Stderr, publishErr)
		return 2
	}
	return 0
}

func saveDecision(path string, decision nublardelivery.Decision) error {
	buffer := new(bytes.Buffer)
	if err := nublardelivery.WriteJSON(buffer, decision); err != nil {
		return fmt.Errorf("encode Nublar decision: %w", err)
	}
	if err := nublaroutput.WriteFile(path, buffer.Bytes()); err != nil {
		return fmt.Errorf("write Nublar decision %s: %w", path, err)
	}
	return nil
}

func saveReceipt(path string, receipt nublardelivery.Receipt) error {
	buffer := new(bytes.Buffer)
	if err := nublardelivery.WriteReceiptJSON(buffer, receipt); err != nil {
		return fmt.Errorf("encode Nublar delivery receipt: %w", err)
	}
	if err := nublaroutput.WriteFile(path, buffer.Bytes()); err != nil {
		return fmt.Errorf("write Nublar delivery receipt %s: %w", path, err)
	}
	return nil
}

func writeRunList(output string, records []nublarrun.Run) error {
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Nublar run list: %w", err)
	}
	data = append(data, '\n')
	if output == "" {
		_, err = os.Stdout.Write(data)
		return err
	}
	if err := nublaroutput.WriteFile(output, data); err != nil {
		return fmt.Errorf("write Nublar run list %s: %w", output, err)
	}
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  nublar workflow validate <path>")
	fmt.Fprintln(os.Stderr, "  nublar run collect --workflow <path> [--root <dir>] [--run-id <id>] [--store <dir>] [--output <path>]")
	fmt.Fprintln(os.Stderr, "  nublar run show --store <dir> --run-id <id> [--output <path>]")
	fmt.Fprintln(os.Stderr, "  nublar run list --store <dir> [--status <passed|failed|error>] [--workflow <id>] [--output <path>]")
	fmt.Fprintln(os.Stderr, "  nublar run decision --store <dir> --run-id <id> [--output <path>]")
	fmt.Fprintln(os.Stderr, "  nublar run deliver --store <dir> --run-id <id> --webhook <url> [--timeout <duration>] [--secret-env <name>] [--receipt <path>]")
	fmt.Fprintln(os.Stderr, "  nublar aggregate [--workflow <path> --root <dir>] [--output <path>] <ci-result> [<ci-result> ...]")
}
