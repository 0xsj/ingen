// Package githubchecks publishes Nublar decisions as GitHub check runs.
package githubchecks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ingen/nublar/internal/delivery"
)

const transport = "github-checks"

const defaultAPIBase = "https://api.github.com"

// Config configures a GitHub Checks publisher. Repository is an owner/name
// pair and HeadSHA identifies the commit to which the check is attached.
type Config struct {
	Repository string
	HeadSHA    string
	CheckName  string
	Token      string
	DetailsURL string
	BaseURL    string
	Client     *http.Client

	// MaxAttempts includes the first request. A zero value uses three attempts.
	MaxAttempts int
	// RetryDelay is the initial delay between transient retries. A zero value
	// disables waiting, which is useful for tests and caller-owned backoff.
	RetryDelay time.Duration
}

// Publisher publishes one decision to the GitHub Checks API.
type Publisher struct {
	owner       string
	repo        string
	checkName   string
	headSHA     string
	token       string
	detailsURL  string
	baseURL     *url.URL
	client      *http.Client
	maxAttempts int
	retryDelay  time.Duration
}

var _ delivery.Publisher = (*Publisher)(nil)

// New creates a GitHub Checks publisher. The token is intentionally accepted
// as configuration rather than read from the environment so callers can own
// secret injection and avoid putting credentials in command arguments.
func New(config Config) (*Publisher, error) {
	repository := strings.TrimSpace(config.Repository)
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return nil, fmt.Errorf("GitHub Checks repository must be owner/name")
	}
	headSHA := strings.TrimSpace(config.HeadSHA)
	if headSHA == "" || strings.ContainsAny(headSHA, "/?#") {
		return nil, fmt.Errorf("GitHub Checks head SHA is required")
	}
	checkName := strings.TrimSpace(config.CheckName)
	if checkName == "" {
		return nil, fmt.Errorf("GitHub Checks check name is required")
	}
	baseURL := strings.TrimSpace(config.BaseURL)
	if baseURL == "" {
		baseURL = defaultAPIBase
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, fmt.Errorf("GitHub Checks API base URL must be an absolute HTTP(S) URL without credentials or fragment")
	}
	if config.Client == nil {
		config.Client = &http.Client{CheckRedirect: func(request *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}}
	}
	maxAttempts := config.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	return &Publisher{
		owner:       parts[0],
		repo:        parts[1],
		checkName:   checkName,
		headSHA:     headSHA,
		token:       strings.TrimSpace(config.Token),
		detailsURL:  strings.TrimSpace(config.DetailsURL),
		baseURL:     parsed,
		client:      config.Client,
		maxAttempts: maxAttempts,
		retryDelay:  config.RetryDelay,
	}, nil
}

// Publish creates or updates the check run representing decision. Repeated
// delivery of one Nublar run finds the remote check by external_id and updates
// it instead of creating another check.
func (p *Publisher) Publish(ctx context.Context, decision delivery.Decision) (delivery.Receipt, error) {
	if p == nil || p.baseURL == nil || p.client == nil {
		return delivery.Receipt{}, fmt.Errorf("GitHub Checks publisher is not configured")
	}
	if err := decision.Validate(); err != nil {
		return delivery.Receipt{}, err
	}
	receipt := delivery.Receipt{
		Schema:      delivery.ReceiptSchema,
		RunID:       decision.RunID,
		Transport:   transport,
		Status:      "failed",
		AttemptedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if p.token == "" {
		err := fmt.Errorf("GitHub Checks token is required")
		receipt.Error = err.Error()
		return receipt, err
	}

	remoteID, found, err := p.findExisting(ctx, decision.RunID)
	if err != nil {
		receipt.Error = err.Error()
		return receipt, err
	}
	body, err := json.Marshal(p.requestBody(decision, found))
	if err != nil {
		err = fmt.Errorf("encode GitHub Checks request: %w", err)
		receipt.Error = err.Error()
		return receipt, err
	}
	path := p.checkRunsPath()
	method := http.MethodPost
	if found {
		method = http.MethodPatch
		path = p.checkRunPath(remoteID)
	}
	var response checkRun
	statusCode, err := p.doJSON(ctx, method, path, body, &response)
	if err != nil {
		receipt.HTTPStatus = statusCode
		receipt.Error = err.Error()
		return receipt, err
	}
	if response.ID <= 0 || response.Name != p.checkName || response.ExternalID != decision.RunID {
		err = fmt.Errorf("GitHub Checks API returned an unexpected check run")
		receipt.HTTPStatus = statusCode
		receipt.Error = err.Error()
		return receipt, err
	}
	receipt.HTTPStatus = statusCode
	receipt.Status = "accepted"
	return receipt, nil
}

type checkRun struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	ExternalID string `json:"external_id"`
}

type checkRunList struct {
	CheckRuns []checkRun `json:"check_runs"`
}

type checkOutput struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Text    string `json:"text"`
}

type createRequest struct {
	Name        string      `json:"name"`
	HeadSHA     string      `json:"head_sha"`
	DetailsURL  string      `json:"details_url,omitempty"`
	ExternalID  string      `json:"external_id"`
	Status      string      `json:"status"`
	StartedAt   string      `json:"started_at"`
	CompletedAt string      `json:"completed_at"`
	Conclusion  string      `json:"conclusion"`
	Output      checkOutput `json:"output"`
}

type updateRequest struct {
	Name        string      `json:"name"`
	DetailsURL  string      `json:"details_url,omitempty"`
	ExternalID  string      `json:"external_id"`
	Status      string      `json:"status"`
	StartedAt   string      `json:"started_at"`
	CompletedAt string      `json:"completed_at"`
	Conclusion  string      `json:"conclusion"`
	Output      checkOutput `json:"output"`
}

func (p *Publisher) requestBody(decision delivery.Decision, updating bool) interface{} {
	conclusion := githubConclusion(decision.Status)
	output := p.output(decision)
	if updating {
		return updateRequest{
			Name:        p.checkName,
			DetailsURL:  p.detailsURL,
			ExternalID:  decision.RunID,
			Status:      "completed",
			StartedAt:   decision.CreatedAt,
			CompletedAt: decision.CompletedAt,
			Conclusion:  conclusion,
			Output:      output,
		}
	}
	return createRequest{
		Name:        p.checkName,
		HeadSHA:     p.headSHA,
		DetailsURL:  p.detailsURL,
		ExternalID:  decision.RunID,
		Status:      "completed",
		StartedAt:   decision.CreatedAt,
		CompletedAt: decision.CompletedAt,
		Conclusion:  conclusion,
		Output:      output,
	}
}

func githubConclusion(status string) string {
	if status == "passed" {
		return "success"
	}
	return "failure"
}

func (p *Publisher) output(decision delivery.Decision) checkOutput {
	var text strings.Builder
	fmt.Fprintf(&text, "Nublar run ID: `%s`\n\n", decision.RunID)
	fmt.Fprintf(&text, "Workflow: `%s`\n\n", decision.Workflow.ID)
	if decision.Correlation != nil {
		fmt.Fprintf(&text, "Correlation: `%s/%s` attempt `%d`\n\n", decision.Correlation.System, decision.Correlation.ID, decision.Correlation.Attempt)
	}
	text.WriteString("Checks:\n\n")
	for _, check := range decision.Checks {
		fmt.Fprintf(&text, "- `%s` (%s, required=%t): **%s**", check.ID, check.Tool, check.Required, check.Status)
		if check.Reason != "" {
			fmt.Fprintf(&text, " — %s", check.Reason)
		}
		if check.Result != nil {
			fmt.Fprintf(&text, " — result `%s` sha256 `%s`", check.Result.Path, check.Result.SHA256)
		}
		text.WriteString("\n")
	}
	if len(decision.Warnings) > 0 {
		text.WriteString("\nWarnings:\n\n")
		for _, issue := range decision.Warnings {
			fmt.Fprintf(&text, "- %s\n", issue.Reason)
		}
	}
	if len(decision.Errors) > 0 {
		text.WriteString("\nErrors:\n\n")
		for _, issue := range decision.Errors {
			fmt.Fprintf(&text, "- %s\n", issue.Reason)
		}
	}
	return checkOutput{
		Title:   fmt.Sprintf("Nublar %s (exit code %d)", decision.Status, decision.ExitCode),
		Summary: fmt.Sprintf("Nublar decision: **%s**; exit code `%d`.", decision.Status, decision.ExitCode),
		Text:    text.String(),
	}
}

func (p *Publisher) findExisting(ctx context.Context, runID string) (int64, bool, error) {
	query := url.Values{}
	query.Set("check_name", p.checkName)
	query.Set("per_page", "100")
	var listed checkRunList
	_, err := p.doJSON(ctx, http.MethodGet, p.checkRunsPath()+"?"+query.Encode(), nil, &listed)
	if err != nil {
		return 0, false, err
	}
	for _, check := range listed.CheckRuns {
		if check.ID > 0 && check.Name == p.checkName && check.ExternalID == runID {
			return check.ID, true, nil
		}
	}
	return 0, false, nil
}

func (p *Publisher) checkRunsPath() string {
	return fmt.Sprintf("/repos/%s/%s/commits/%s/check-runs", url.PathEscape(p.owner), url.PathEscape(p.repo), url.PathEscape(p.headSHA))
}

func (p *Publisher) checkRunPath(id int64) string {
	return fmt.Sprintf("/repos/%s/%s/check-runs/%d", url.PathEscape(p.owner), url.PathEscape(p.repo), id)
}

func (p *Publisher) doJSON(ctx context.Context, method, path string, body []byte, output ...interface{}) (int, error) {
	var lastStatus int
	for attempt := 1; attempt <= p.maxAttempts; attempt++ {
		request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(p.baseURL.String(), "/")+path, bytes.NewReader(body))
		if err != nil {
			return 0, fmt.Errorf("create GitHub Checks request: %w", err)
		}
		request.Header.Set("Accept", "application/vnd.github+json")
		request.Header.Set("Authorization", "Bearer "+p.token)
		request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if len(body) > 0 {
			request.Header.Set("Content-Type", "application/json")
		}
		response, err := p.client.Do(request)
		if err != nil {
			if attempt < p.maxAttempts && retryableError(ctx, err) {
				if err := p.wait(ctx, attempt); err != nil {
					return 0, err
				}
				continue
			}
			return 0, fmt.Errorf("publish GitHub Checks decision: %w", err)
		}
		lastStatus = response.StatusCode
		contents, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			if attempt < p.maxAttempts && retryableStatus(response.StatusCode) {
				if err := p.wait(ctx, attempt); err != nil {
					return lastStatus, err
				}
				continue
			}
			return lastStatus, fmt.Errorf("read GitHub Checks response: %w", readErr)
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			err = fmt.Errorf("GitHub Checks API returned HTTP %s", response.Status)
			if attempt < p.maxAttempts && retryableStatus(response.StatusCode) {
				if err := p.wait(ctx, attempt); err != nil {
					return lastStatus, err
				}
				continue
			}
			return lastStatus, err
		}
		if len(output) > 0 {
			if err := json.Unmarshal(contents, output[0]); err != nil {
				return lastStatus, fmt.Errorf("decode GitHub Checks response: %w", err)
			}
		}
		return lastStatus, nil
	}
	return lastStatus, fmt.Errorf("GitHub Checks request exhausted retries")
}

func retryableError(ctx context.Context, err error) bool {
	return ctx.Err() == nil && err != nil
}

func retryableStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500
}

func (p *Publisher) wait(ctx context.Context, attempt int) error {
	if p.retryDelay <= 0 {
		return nil
	}
	delay := p.retryDelay
	for i := 1; i < attempt && delay < time.Second; i++ {
		delay *= 2
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
