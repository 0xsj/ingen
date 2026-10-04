package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"ingen/herdr-sentinel/internal/nativesession"
)

func nativeSessionAssessCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel session native-assess", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("root", "", "absolute project root")
	path := flags.String("path", "", "native session journal path relative to project root")
	format := flags.String("format", "json", "output format: json or text")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *root == "" || !filepath.IsAbs(*root) || *path == "" || (*format != "json" && *format != "text") {
		fmt.Fprintln(os.Stderr, "native-assess requires --root <absolute-dir>, --path <relative-journal>, and --format json|text")
		return 2
	}
	assessment, err := nativesession.Assess(*root, *path, time.Time{})
	if err != nil {
		fmt.Fprintln(os.Stderr, "native assessment failed:", err)
		return 2
	}
	if *format == "json" {
		if err := assessment.WriteJSON(os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "write native assessment:", err)
			return 2
		}
	} else {
		fmt.Printf("native-session: %s\nstate: %s\nassessment: %s\njournal-sha256: %s\nsnapshot: %s\nexecution-lease: %s\nreceipt: %s\nterminal-evidence: %s\nrecommended-action: %s\nrecommendation: %s\n",
			assessment.SessionID, assessment.State, assessment.Status, assessment.JournalSHA256,
			assessment.SnapshotConsistency, assessment.ExecutionLease.Status, assessment.Receipt.Status,
			assessment.TerminalEvidence.Status, assessment.RecommendedAction, assessment.Recommendation)
	}
	if assessment.Status == nativesession.AssessmentUncertain {
		return 1
	}
	return 0
}
