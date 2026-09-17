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
	nublargithubchecks "ingen/nublar/internal/delivery/githubchecks"
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
	case "receipt":
		if len(args) < 2 || args[1] != "list" {
			usage()
			return 2
		}
		return receiptListCommand(args[2:])
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
	correlationSystem := flags.String("external-system", "", "optional external CI system name")
	correlationID := flags.String("external-id", "", "optional external CI correlation ID")
	attempt := flags.Int64("attempt", 0, "optional positive external CI attempt number")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *workflowPath == "" || len(flags.Args()) > 0 {
		fmt.Fprintln(os.Stderr, "run collect requires --workflow and does not accept positional arguments")
		return 2
	}
	correlation, err := parseCorrelation(*correlationSystem, *correlationID, *attempt)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
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
	report, err := nublarrun.CollectWorkflowFileWithCorrelation(*workflowPath, *root, *runID, correlation)
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

func parseCorrelation(system, id string, attempt int64) (*nublarrun.Correlation, error) {
	if system == "" && id == "" && attempt == 0 {
		return nil, nil
	}
	correlation := &nublarrun.Correlation{System: system, ID: id, Attempt: attempt}
	if err := correlation.Validate(); err != nil {
		return nil, fmt.Errorf("parse Nublar external correlation: %w", err)
	}
	return correlation, nil
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
	correlationSystem := flags.String("external-system", "", "optional external correlation system filter")
	correlationID := flags.String("external-id", "", "optional external correlation ID filter")
	attempt := flags.Int64("attempt", 0, "optional external correlation attempt filter")
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
	records, err = filterRuns(records, *status, *workflowID, *correlationSystem, *correlationID, *attempt)
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

func filterRuns(records []nublarrun.Run, status, workflowID, correlationSystem, correlationID string, attempt int64) ([]nublarrun.Run, error) {
	status = strings.TrimSpace(status)
	workflowID = strings.TrimSpace(workflowID)
	correlationSystem = strings.TrimSpace(correlationSystem)
	correlationID = strings.TrimSpace(correlationID)
	if status != "" {
		if _, err := ciresult.ExitCodeForStatus(status); err != nil {
			return nil, fmt.Errorf("Nublar run list status filter: %w", err)
		}
	}
	if attempt < 0 {
		return nil, fmt.Errorf("Nublar run list attempt filter must not be negative")
	}
	filtered := make([]nublarrun.Run, 0, len(records))
	for _, record := range records {
		if status != "" && record.Status != status {
			continue
		}
		if workflowID != "" && record.Workflow.ID != workflowID {
			continue
		}
		if correlationSystem != "" && (record.Correlation == nil || record.Correlation.System != correlationSystem) {
			continue
		}
		if correlationID != "" && (record.Correlation == nil || record.Correlation.ID != correlationID) {
			continue
		}
		if attempt > 0 && (record.Correlation == nil || record.Correlation.Attempt != attempt) {
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
	switch transport := deliveryTransportArg(args); transport {
	case "github-checks":
		return githubChecksDeliverCommand(args)
	case "http-webhook":
		// The original webhook flags remain the default delivery surface.
	default:
		fmt.Fprintf(os.Stderr, "unsupported delivery transport %q\n", transport)
		return 2
	}
	return deliverCommandWithFactory(args, func(endpoint string, secret []byte) (nublardelivery.Publisher, error) {
		return nublarwebhook.NewWithSecret(endpoint, nil, secret)
	})
}

func deliveryTransportArg(args []string) string {
	for index, arg := range args {
		if arg == "--transport" && index+1 < len(args) {
			return args[index+1]
		}
		if strings.HasPrefix(arg, "--transport=") {
			return strings.TrimPrefix(arg, "--transport=")
		}
	}
	return "http-webhook"
}

func deliverCommandWithFactory(args []string, newPublisher webhookPublisherFactory) int {
	flags := flag.NewFlagSet("run deliver", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	transport := flags.String("transport", "http-webhook", "delivery transport")
	storeRoot := flags.String("store", "", "filesystem store root")
	runID := flags.String("run-id", "", "Nublar run ID")
	endpoint := flags.String("webhook", "", "HTTP(S) webhook endpoint")
	timeout := flags.Duration("timeout", 30*time.Second, "maximum time for one webhook delivery")
	secretEnv := flags.String("secret-env", "", "environment variable containing optional webhook HMAC secret")
	receiptPath := flags.String("receipt", "", "path for the delivery receipt; no receipt file when empty")
	receiptStoreRoot := flags.String("receipt-store", "", "optional filesystem receipt store; stores every publisher outcome")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *transport != "http-webhook" || *storeRoot == "" || *runID == "" || *endpoint == "" || *timeout <= 0 || len(flags.Args()) > 0 {
		fmt.Fprintln(os.Stderr, "run deliver requires --store, --run-id, --webhook, and a positive --timeout")
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
	return deliverStoredRun(*storeRoot, *runID, *timeout, *receiptPath, *receiptStoreRoot, publisher)
}

type githubChecksPublisherFactory func(nublargithubchecks.Config) (nublardelivery.Publisher, error)

func githubChecksDeliverCommand(args []string) int {
	return githubChecksDeliverCommandWithFactory(args, func(config nublargithubchecks.Config) (nublardelivery.Publisher, error) {
		return nublargithubchecks.New(config)
	})
}

func githubChecksDeliverCommandWithFactory(args []string, newPublisher githubChecksPublisherFactory) int {
	flags := flag.NewFlagSet("run deliver", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	transport := flags.String("transport", "github-checks", "delivery transport")
	storeRoot := flags.String("store", "", "filesystem store root")
	runID := flags.String("run-id", "", "Nublar run ID")
	repository := flags.String("repository", "", "GitHub repository in owner/name form")
	headSHA := flags.String("head-sha", "", "GitHub commit SHA for the check run")
	checkName := flags.String("check-name", "", "GitHub check name; defaults to Nublar / <workflow-id>")
	detailsURL := flags.String("details-url", "", "optional GitHub check details URL")
	tokenEnv := flags.String("token-env", "GITHUB_TOKEN", "environment variable containing the GitHub token")
	apiBaseURL := flags.String("api-base-url", "", "GitHub Checks API base URL; defaults to api.github.com")
	timeout := flags.Duration("timeout", 30*time.Second, "maximum time for one delivery")
	maxAttempts := flags.Int("max-attempts", 3, "maximum attempts per GitHub API request")
	retryDelay := flags.Duration("retry-delay", 100*time.Millisecond, "initial delay between transient retries")
	receiptPath := flags.String("receipt", "", "path for the delivery receipt; no receipt file when empty")
	receiptStoreRoot := flags.String("receipt-store", "", "optional filesystem receipt store; stores every publisher outcome")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *transport != "github-checks" || *storeRoot == "" || *runID == "" || *repository == "" || *headSHA == "" || *timeout <= 0 || *maxAttempts <= 0 || *retryDelay < 0 || len(flags.Args()) > 0 {
		fmt.Fprintln(os.Stderr, "GitHub Checks delivery requires --transport github-checks, --store, --run-id, --repository, --head-sha, and positive timeout/attempts")
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
	name := strings.TrimSpace(*checkName)
	if name == "" {
		name = "Nublar / " + decision.Workflow.ID
	}
	var token string
	if *tokenEnv != "" {
		token, _ = os.LookupEnv(*tokenEnv)
	}
	publisher, err := newPublisher(nublargithubchecks.Config{
		Repository:  *repository,
		HeadSHA:     *headSHA,
		CheckName:   name,
		Token:       token,
		DetailsURL:  *detailsURL,
		BaseURL:     *apiBaseURL,
		MaxAttempts: *maxAttempts,
		RetryDelay:  *retryDelay,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return deliverDecision(decision, *timeout, *receiptPath, *receiptStoreRoot, publisher)
}

func deliverStoredRun(storeRoot, runID string, timeout time.Duration, receiptPath, receiptStoreRoot string, publisher nublardelivery.Publisher) int {
	store, err := nublarstore.New(storeRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	record, err := store.Load(runID)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	decision, err := nublardelivery.Project(record)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return deliverDecision(decision, timeout, receiptPath, receiptStoreRoot, publisher)
}

func deliverDecision(decision nublardelivery.Decision, timeout time.Duration, receiptPath, receiptStoreRoot string, publisher nublardelivery.Publisher) int {
	var receiptStore *nublarstore.ReceiptStore
	var err error
	if receiptStoreRoot != "" {
		receiptStore, err = nublarstore.NewReceiptStore(receiptStoreRoot)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	receipt, publishErr := publisher.Publish(ctx, decision)
	if receiptPath != "" {
		if err := saveReceipt(receiptPath, receipt); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}
	if receiptStore != nil {
		if err := receiptStore.Save(receipt); err != nil {
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

func receiptListCommand(args []string) int {
	flags := flag.NewFlagSet("run receipt list", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	receiptStoreRoot := flags.String("receipt-store", "", "filesystem receipt store root")
	runID := flags.String("run-id", "", "optional Nublar run ID filter")
	status := flags.String("status", "", "optional receipt status filter: accepted or failed")
	transport := flags.String("transport", "", "optional delivery transport filter")
	output := flags.String("output", "", "path for the JSON receipt list; stdout when empty")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *receiptStoreRoot == "" || len(flags.Args()) > 0 {
		fmt.Fprintln(os.Stderr, "run receipt list requires --receipt-store and does not accept positional arguments")
		return 2
	}
	store, err := nublarstore.NewReceiptStore(*receiptStoreRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	receipts, err := store.List()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	receipts, err = filterReceipts(receipts, *runID, *status, *transport)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if err := writeReceiptList(*output, receipts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return 0
}

func filterReceipts(receipts []nublardelivery.Receipt, runID, status, transport string) ([]nublardelivery.Receipt, error) {
	runID = strings.TrimSpace(runID)
	status = strings.TrimSpace(status)
	transport = strings.TrimSpace(transport)
	if status != "" && status != "accepted" && status != "failed" {
		return nil, fmt.Errorf("Nublar receipt list status filter has unsupported status %q", status)
	}
	filtered := make([]nublardelivery.Receipt, 0, len(receipts))
	for _, receipt := range receipts {
		if runID != "" && receipt.RunID != runID {
			continue
		}
		if status != "" && receipt.Status != status {
			continue
		}
		if transport != "" && receipt.Transport != transport {
			continue
		}
		filtered = append(filtered, receipt)
	}
	return filtered, nil
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

func writeReceiptList(output string, receipts []nublardelivery.Receipt) error {
	data, err := json.MarshalIndent(receipts, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Nublar receipt list: %w", err)
	}
	data = append(data, '\n')
	if output == "" {
		_, err = os.Stdout.Write(data)
		return err
	}
	if err := nublaroutput.WriteFile(output, data); err != nil {
		return fmt.Errorf("write Nublar receipt list %s: %w", output, err)
	}
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  nublar workflow validate <path>")
	fmt.Fprintln(os.Stderr, "  nublar run collect --workflow <path> [--root <dir>] [--run-id <id>] [--store <dir>] [--output <path>] [--external-system <name> --external-id <id> --attempt <n>]")
	fmt.Fprintln(os.Stderr, "  nublar run show --store <dir> --run-id <id> [--output <path>]")
	fmt.Fprintln(os.Stderr, "  nublar run list --store <dir> [--status <passed|failed|error>] [--workflow <id>] [--external-system <name>] [--external-id <id>] [--attempt <n>] [--output <path>]")
	fmt.Fprintln(os.Stderr, "  nublar run decision --store <dir> --run-id <id> [--output <path>]")
	fmt.Fprintln(os.Stderr, "  nublar run deliver --transport http-webhook --store <dir> --run-id <id> --webhook <url> [--timeout <duration>] [--secret-env <name>] [--receipt <path>] [--receipt-store <dir>]")
	fmt.Fprintln(os.Stderr, "  nublar run deliver --transport github-checks --store <dir> --run-id <id> --repository <owner/name> --head-sha <sha> [--check-name <name>] [--token-env <name>] [--receipt <path>] [--receipt-store <dir>]")
	fmt.Fprintln(os.Stderr, "  nublar run receipt list --receipt-store <dir> [--run-id <id>] [--status <accepted|failed>] [--transport <name>] [--output <path>]")
	fmt.Fprintln(os.Stderr, "  nublar aggregate [--workflow <path> --root <dir>] [--output <path>] <ci-result> [<ci-result> ...]")
}
