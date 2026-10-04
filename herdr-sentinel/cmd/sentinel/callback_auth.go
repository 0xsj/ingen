package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"ingen/herdr-sentinel/internal/callbackauth"
)

func herdrSignEventsCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel adapter herdr-sign-events", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("root", ".", "project root used to validate referenced artifacts")
	receiptPath := flags.String("receipt", "", "Sentinel run receipt to bind")
	eventsPath := flags.String("events", "", "JSONL file of normalized ingen.herdr-event/v1 events")
	keyPath := flags.String("key", "", "operator-owned 0600 raw 32-byte shared key file")
	sender := flags.String("sender", "", "configured sender identity to authenticate")
	outputPath := flags.String("output", "", "new signed-envelope output path (must not exist)")
	lifetimeSeconds := flags.Int("lifetime-seconds", 300, "signed batch lifetime, 1-300 seconds")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if len(flags.Args()) != 0 || *receiptPath == "" || *eventsPath == "" || *keyPath == "" || *sender == "" || *outputPath == "" || *lifetimeSeconds < 1 || *lifetimeSeconds > int(callbackauth.MaxLifetime/time.Second) {
		usage()
		return 2
	}
	envelope, err := callbackauth.SignFile(*root, *receiptPath, *eventsPath, *keyPath, *sender, *outputPath, time.Now().UTC(), time.Duration(*lifetimeSeconds)*time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("created authenticated Herdr event batch for sender %s (key %s): %s\n", envelope.Sender, envelope.KeyID, *outputPath)
	fmt.Fprintln(os.Stderr, "authentication proves shared-key possession only; it does not attest a Herdr callback or host")
	return 0
}

func herdrAuthEventsCommand(args []string) int {
	flags := flag.NewFlagSet("sentinel adapter herdr-auth-events", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	root := flags.String("root", ".", "project root containing receipt artifact references")
	receiptPath := flags.String("receipt", "", "Sentinel run receipt to update")
	envelopePath := flags.String("envelope", "", "signed ingen.herdr-signed-event-batch/v1 file")
	keyPath := flags.String("key", "", "operator-owned 0600 raw 32-byte shared key file")
	expectedSender := flags.String("expected-sender", "", "locally configured sender identity required for acceptance")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if len(flags.Args()) != 0 || *receiptPath == "" || *envelopePath == "" || *keyPath == "" || *expectedSender == "" {
		usage()
		return 2
	}
	appended, err := callbackauth.IngestFile(*receiptPath, *envelopePath, *root, *keyPath, *expectedSender, time.Now().UTC())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("authenticated Herdr event batch: %d new events\n", appended)
	fmt.Fprintln(os.Stderr, "authentication proves shared-key possession only; it does not attest a Herdr callback or host")
	return 0
}
