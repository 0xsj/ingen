package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"

	"ingen/sorna/internal/runner"
)

// replayRequestFingerprint captures the public request intent observed for a
// rule. It deliberately excludes host, port, scheme, and response data so
// equivalent subjects can be compared while request-shape drift remains
// visible.
func replayRequestFingerprint(rule runner.RuleResult) (string, bool) {
	if !requestPresent(rule.Request) {
		return "", false
	}
	sequence := struct {
		Setup  []replayRequestIntent `json:"setup"`
		Target replayRequestIntent   `json:"target"`
	}{
		Setup:  make([]replayRequestIntent, 0, len(rule.Setup)),
		Target: normalizeReplayRequest(rule.Request),
	}
	for _, step := range rule.Setup {
		if !requestPresent(step.Request) {
			return "", false
		}
		sequence.Setup = append(sequence.Setup, normalizeReplayRequest(step.Request))
	}
	encoded, err := json.Marshal(sequence)
	if err != nil {
		return "", false
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), true
}

type replayRequestIntent struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	Query   string `json:"query,omitempty"`
	HasBody bool   `json:"has_body"`
	Body    any    `json:"body,omitempty"`
}

func normalizeReplayRequest(request runner.Request) replayRequestIntent {
	intent := replayRequestIntent{Method: strings.ToUpper(strings.TrimSpace(request.Method))}
	parsed, err := url.Parse(request.URL)
	if err != nil {
		intent.Path = request.URL
	} else {
		intent.Path = parsed.EscapedPath()
		if intent.Path == "" {
			intent.Path = "/"
		}
		intent.Query = parsed.RawQuery
	}
	if request.Body != nil {
		intent.HasBody = true
		intent.Body = request.Body
	}
	return intent
}

func requestPresent(request runner.Request) bool {
	return strings.TrimSpace(request.Method) != "" && strings.TrimSpace(request.URL) != ""
}
