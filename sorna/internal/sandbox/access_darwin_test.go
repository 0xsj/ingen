//go:build darwin

package sandbox

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestParseAccessOutputScopesStructuredEventsToProcess(t *testing.T) {
	output := []byte(fmt.Sprintf(`{"timestamp":"2026-09-14 08:56:34.966769+0300","eventMessage":"Sandbox: sorna(%d) deny(1) file-read-data /project/implementation.go"}
{"timestamp":"2026-09-14 08:56:34.966770+0300","eventMessage":"Sandbox: other(%d) deny(1) file-read-data /other/secret.go"}
{"count":2}
`, 1234, 5678))
	events, parseErrors := parseAccessOutput(output, 1234)
	if parseErrors != 0 || len(events) != 1 {
		t.Fatalf("events = %+v, parse errors = %d; want one scoped event", events, parseErrors)
	}
	if events[0].PID != 1234 || events[0].Decision != "deny" {
		t.Fatalf("event = %+v, want PID 1234 deny decision", events[0])
	}
}

func TestParseAccessEventNormalizesSeatbeltMessage(t *testing.T) {
	event, err := parseAccessEvent(macOSLogEvent{
		Timestamp:    "2026-09-14 08:56:34.966769+0300",
		EventMessage: "Sandbox: sorna(1234) deny(1) file-read-data /project/implementation.go",
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.Timestamp.Equal(time.Time{}) || event.Process != "sorna" || event.PID != 1234 || event.Decision != "deny" || event.Operation != "file-read-data" || event.Resource != "/project/implementation.go" {
		t.Fatalf("event = %+v, want normalized Seatbelt fields", event)
	}
}

func TestParseAccessEventRejectsDuplicateReportEnvelope(t *testing.T) {
	_, err := parseAccessEvent(macOSLogEvent{
		Timestamp:    "2026-09-14T08:56:34.966769+03:00",
		EventMessage: "1 duplicate report for Sandbox: sorna(1234) deny(1) file-read-data /project/implementation.go",
	})
	if err == nil || !strings.Contains(err.Error(), "unrecognized sandbox event") {
		t.Fatalf("parseAccessEvent() = %v, want duplicate envelope rejection", err)
	}
}
