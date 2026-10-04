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

const (
	listPageSize        = 100
	maxListPages        = 10
	maxResponseBodySize = 8 << 20
	maxRequestTimeout   = 30 * time.Second
	maxPublishDuration  = 2 * time.Minute
	maxRetryAttempts    = 5
	maxRetryDelay       = 5 * time.Second
)

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
	client := http.Client{}
	if config.Client != nil {
		client = *config.Client
	}
	if client.Timeout <= 0 || client.Timeout > maxRequestTimeout {
		client.Timeout = maxRequestTimeout
	}
	// Never forward the bearer credential or mutation request through a
	// redirect, even when a caller supplied a client with redirect behavior.
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	maxAttempts := config.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	} else if maxAttempts > maxRetryAttempts {
		maxAttempts = maxRetryAttempts
	}
	if config.RetryDelay > maxRetryDelay {
		config.RetryDelay = maxRetryDelay
	}
	return &Publisher{
		owner:       parts[0],
		repo:        parts[1],
		checkName:   checkName,
		headSHA:     headSHA,
		token:       strings.TrimSpace(config.Token),
		detailsURL:  strings.TrimSpace(config.DetailsURL),
		baseURL:     parsed,
		client:      &client,
		maxAttempts: maxAttempts,
		retryDelay:  config.RetryDelay,
	}, nil
}

// Publish creates or updates the check run representing decision. Repeated
// delivery of one Nublar run finds the remote check by external_id and updates
// it instead of creating another check. Reconciliation scans at most the
// configured window of at most 1000 matching list results; upstream visibility
// or retention may omit older checks. This is not durable exactly-once delivery,
// and concurrent publishers can still race to create the same check.
func (p *Publisher) Publish(ctx context.Context, decision delivery.Decision) (delivery.Receipt, error) {
	if p == nil || p.baseURL == nil || p.client == nil {
		return delivery.Receipt{}, fmt.Errorf("GitHub Checks publisher is not configured")
	}
	if err := decision.Validate(); err != nil {
		return delivery.Receipt{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, maxPublishDuration)
	defer cancel()
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

	existing, found, err := p.findExisting(ctx, decision.RunID)
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
	path := p.createCheckRunPath()
	method := http.MethodPost
	if found {
		method = http.MethodPatch
		path = p.checkRunPath(existing.ID)
	}
	var response checkRun
	var statusCode int
	if method == http.MethodPost {
		var ambiguous bool
		statusCode, _, ambiguous, _, err = p.doJSONOnce(ctx, method, p.requestURL(path), body, &response)
		if err != nil && ambiguous {
			reconciledStatus, reconcileErr := p.reconcileCreate(ctx, decision, statusCode, err)
			err = reconcileErr
			if err == nil {
				receipt.HTTPStatus = reconciledStatus
				receipt.Status = "accepted"
				return receipt, nil
			}
		}
	} else {
		statusCode, _, err = p.doJSON(ctx, method, path, body, &response)
	}
	if err != nil {
		receipt.HTTPStatus = statusCode
		receipt.Error = err.Error()
		return receipt, err
	}
	wantID := int64(0)
	if found {
		wantID = existing.ID
	}
	if !p.validAcknowledgement(response, decision, wantID) {
		err = fmt.Errorf("GitHub Checks API returned a check run with unexpected identity or commit")
		if method == http.MethodPost {
			reconciledStatus, reconcileErr := p.reconcileCreate(ctx, decision, statusCode, err)
			err = reconcileErr
			if err == nil {
				receipt.HTTPStatus = reconciledStatus
				receipt.Status = "accepted"
				return receipt, nil
			}
		}
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
	HeadSHA    string `json:"head_sha"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

type checkRunList struct {
	TotalCount *int       `json:"total_count"`
	CheckRuns  []checkRun `json:"check_runs"`
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

func (p *Publisher) findExisting(ctx context.Context, runID string) (checkRun, bool, error) {
	query := url.Values{}
	query.Set("check_name", p.checkName)
	query.Set("filter", "all")
	query.Set("per_page", fmt.Sprint(listPageSize))
	query.Set("page", "1")
	listPath := p.checkRunsPath()
	currentURL := p.requestURL(listPath + "?" + query.Encode())
	var expectedTotal = -1
	var scanned int
	var matched *checkRun
	for page := 1; page <= maxListPages; page++ {
		var listed checkRunList
		_, headers, err := p.doJSONURL(ctx, http.MethodGet, currentURL, nil, &listed)
		if err != nil {
			return checkRun{}, false, err
		}
		if listed.TotalCount == nil || *listed.TotalCount < 0 || len(listed.CheckRuns) > listPageSize {
			return checkRun{}, false, fmt.Errorf("GitHub Checks API returned an incomplete or oversized list page")
		}
		if expectedTotal < 0 {
			expectedTotal = *listed.TotalCount
			if expectedTotal > listPageSize*maxListPages {
				return checkRun{}, false, fmt.Errorf("GitHub Checks list has %d results, beyond the bounded %d-result reconciliation limit", expectedTotal, listPageSize*maxListPages)
			}
		} else if *listed.TotalCount != expectedTotal {
			return checkRun{}, false, fmt.Errorf("GitHub Checks list changed during pagination; refusing an ambiguous identity result")
		}
		scanned += len(listed.CheckRuns)
		if scanned > expectedTotal {
			return checkRun{}, false, fmt.Errorf("GitHub Checks pagination returned more runs than its declared total")
		}
		for _, check := range listed.CheckRuns {
			if check.Name != p.checkName || check.ExternalID != runID {
				continue
			}
			if check.ID <= 0 || check.HeadSHA != p.headSHA {
				return checkRun{}, false, fmt.Errorf("GitHub Checks API returned a matching run with unexpected ID or commit")
			}
			if matched != nil {
				return checkRun{}, false, fmt.Errorf("multiple GitHub check runs match this name and external ID")
			}
			copy := check
			matched = &copy
		}

		next, hasNext, err := nextPageURL(headers.Get("Link"), p.baseURL, listPath, query, page)
		if err != nil {
			return checkRun{}, false, err
		}
		if !hasNext {
			if scanned != expectedTotal {
				return checkRun{}, false, fmt.Errorf("GitHub Checks pagination ended after %d of %d listed runs", scanned, expectedTotal)
			}
			if matched != nil {
				return *matched, true, nil
			}
			return checkRun{}, false, nil
		}
		if scanned >= expectedTotal {
			return checkRun{}, false, fmt.Errorf("GitHub Checks pagination continued beyond its declared total")
		}
		if page == maxListPages {
			return checkRun{}, false, fmt.Errorf("GitHub Checks pagination exceeds the bounded %d-page reconciliation limit", maxListPages)
		}
		currentURL = next
	}
	return checkRun{}, false, fmt.Errorf("GitHub Checks pagination did not terminate")
}

func (p *Publisher) checkRunsPath() string {
	return fmt.Sprintf("/repos/%s/%s/commits/%s/check-runs", url.PathEscape(p.owner), url.PathEscape(p.repo), url.PathEscape(p.headSHA))
}

func (p *Publisher) createCheckRunPath() string {
	return fmt.Sprintf("/repos/%s/%s/check-runs", url.PathEscape(p.owner), url.PathEscape(p.repo))
}

func (p *Publisher) checkRunPath(id int64) string {
	return fmt.Sprintf("/repos/%s/%s/check-runs/%d", url.PathEscape(p.owner), url.PathEscape(p.repo), id)
}

func (p *Publisher) requestURL(path string) string {
	return strings.TrimRight(p.baseURL.String(), "/") + path
}

func (p *Publisher) doJSON(ctx context.Context, method, path string, body []byte, output ...interface{}) (int, http.Header, error) {
	return p.doJSONURL(ctx, method, p.requestURL(path), body, output...)
}

func (p *Publisher) doJSONURL(ctx context.Context, method, requestURL string, body []byte, output ...interface{}) (int, http.Header, error) {
	var lastStatus int
	maxAttempts := p.maxAttempts
	if method != http.MethodGet && method != http.MethodPatch {
		maxAttempts = 1
	}
	var lastHeaders http.Header
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		status, headers, _, retryable, err := p.doJSONOnce(ctx, method, requestURL, body, output...)
		lastStatus = status
		lastHeaders = headers
		if err == nil {
			return status, headers, nil
		}
		if attempt < maxAttempts && retryable {
			if err := p.wait(ctx, attempt); err != nil {
				return lastStatus, lastHeaders, err
			}
			continue
		}
		return lastStatus, lastHeaders, err
	}
	return lastStatus, lastHeaders, fmt.Errorf("GitHub Checks request exhausted retries")
}

func (p *Publisher) doJSONOnce(ctx context.Context, method, requestURL string, body []byte, output ...interface{}) (status int, headers http.Header, ambiguous, retryable bool, err error) {
	request, err := http.NewRequestWithContext(ctx, method, requestURL, bytes.NewReader(body))
	if err != nil {
		return 0, nil, false, false, fmt.Errorf("create GitHub Checks request failed")
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+p.token)
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := p.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, true, false, fmt.Errorf("GitHub Checks request canceled: %v", ctx.Err())
		}
		return 0, nil, true, true, fmt.Errorf("GitHub Checks request failed")
	}
	status = response.StatusCode
	headers = response.Header.Clone()
	contents, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBodySize+1))
	_ = response.Body.Close()
	if readErr != nil {
		return status, headers, true, retryableStatus(status), fmt.Errorf("read GitHub Checks response failed")
	}
	if int64(len(contents)) > maxResponseBodySize {
		return status, headers, true, false, fmt.Errorf("GitHub Checks response exceeded the %d-byte limit", maxResponseBodySize)
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return status, headers, retryableStatus(status), retryableStatus(status), fmt.Errorf("GitHub Checks API returned HTTP status %d", status)
	}
	if len(output) > 0 {
		if err := json.Unmarshal(contents, output[0]); err != nil {
			return status, headers, true, false, fmt.Errorf("decode GitHub Checks response failed")
		}
	}
	return status, headers, false, false, nil
}

func (p *Publisher) validAcknowledgement(response checkRun, decision delivery.Decision, expectedID int64) bool {
	return response.ID > 0 && (expectedID == 0 || response.ID == expectedID) && response.Name == p.checkName && response.ExternalID == decision.RunID && response.HeadSHA == p.headSHA && response.Status == "completed" && response.Conclusion == githubConclusion(decision.Status)
}

func (p *Publisher) reconcileCreate(ctx context.Context, decision delivery.Decision, status int, createErr error) (int, error) {
	remote, found, reconcileErr := p.findExisting(ctx, decision.RunID)
	if reconcileErr != nil {
		return 0, fmt.Errorf("GitHub Checks create outcome is uncertain after HTTP %d; reconciliation failed: %w", status, reconcileErr)
	}
	if found {
		body, err := json.Marshal(p.requestBody(decision, true))
		if err != nil {
			return 0, fmt.Errorf("GitHub Checks create outcome is uncertain after HTTP %d; could not encode reconciliation update", status)
		}
		var updated checkRun
		updateStatus, _, err := p.doJSON(ctx, http.MethodPatch, p.checkRunPath(remote.ID), body, &updated)
		if err != nil {
			return updateStatus, fmt.Errorf("GitHub Checks create outcome is uncertain after HTTP %d; reconciliation update failed: %w", status, err)
		}
		if !p.validAcknowledgement(updated, decision, remote.ID) {
			return updateStatus, fmt.Errorf("GitHub Checks create outcome is uncertain after HTTP %d; reconciliation update returned an unexpected identity, commit, or decision", status)
		}
		return updateStatus, nil
	}
	return 0, fmt.Errorf("GitHub Checks create outcome is uncertain after HTTP %d: %v; reconciliation found no matching check; refusing to retry POST", status, createErr)
}

// nextPageURL extracts and validates the documented Link rel=next target.
// The target must remain on the configured origin and exact list endpoint,
// with the original filters and the next sequential page number.
func nextPageURL(header string, baseURL *url.URL, listPath string, query url.Values, currentPage int) (string, bool, error) {
	var next string
	for _, segment := range strings.Split(header, ",") {
		parts := strings.Split(segment, ";")
		if len(parts) < 2 {
			continue
		}
		relation := ""
		for _, parameter := range parts[1:] {
			key, value, ok := strings.Cut(strings.TrimSpace(parameter), "=")
			if ok && strings.EqualFold(strings.TrimSpace(key), "rel") {
				relation = strings.Trim(strings.TrimSpace(value), "\"")
			}
		}
		if relation != "next" {
			continue
		}
		link := strings.TrimSpace(parts[0])
		if len(link) < 3 || link[0] != '<' || link[len(link)-1] != '>' {
			return "", false, fmt.Errorf("GitHub Checks API returned a malformed next-page link")
		}
		if next != "" {
			return "", false, fmt.Errorf("GitHub Checks API returned multiple next-page links")
		}
		next = link[1 : len(link)-1]
	}
	if next == "" {
		return "", false, nil
	}
	parsed, err := url.Parse(next)
	if err != nil || parsed.Scheme != baseURL.Scheme || !strings.EqualFold(parsed.Host, baseURL.Host) || parsed.User != nil || parsed.Fragment != "" || parsed.Path != expectedBasePath(baseURL, listPath) {
		return "", false, fmt.Errorf("GitHub Checks API returned a next-page link outside the configured endpoint")
	}
	wantQuery := query
	wantPage := fmt.Sprint(currentPage + 1)
	got := parsed.Query()
	if got.Get("page") != wantPage {
		return "", false, fmt.Errorf("GitHub Checks API returned a nonsequential next-page link")
	}
	for _, values := range got {
		if len(values) != 1 {
			return "", false, fmt.Errorf("GitHub Checks API returned duplicate next-page query parameters")
		}
	}
	for key, values := range wantQuery {
		if key == "page" {
			continue
		}
		if len(values) != 1 || got.Get(key) != values[0] {
			return "", false, fmt.Errorf("GitHub Checks API changed a next-page query filter")
		}
	}
	if len(got) != len(wantQuery) {
		return "", false, fmt.Errorf("GitHub Checks API changed the next-page query parameters")
	}
	return parsed.String(), true, nil
}

func expectedBasePath(baseURL *url.URL, endpointPath string) string {
	return strings.TrimRight(baseURL.Path, "/") + endpointPath
}

func retryableStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500
}

func (p *Publisher) wait(ctx context.Context, attempt int) error {
	if p.retryDelay <= 0 {
		return nil
	}
	delay := p.retryDelay
	if delay > maxRetryDelay {
		delay = maxRetryDelay
	}
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
