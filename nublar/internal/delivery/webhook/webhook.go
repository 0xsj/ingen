// Package webhook publishes Nublar decisions to a generic HTTP endpoint.
package webhook

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"ingen/nublar/internal/delivery"
)

type Publisher struct {
	endpoint *url.URL
	client   *http.Client
}

// New creates a generic HTTP webhook publisher. Authentication and retry
// policy are intentionally configured by a future concrete adapter.
func New(endpoint string, client *http.Client) (*Publisher, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return nil, fmt.Errorf("parse Nublar webhook endpoint: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, fmt.Errorf("Nublar webhook endpoint must be an absolute HTTP(S) URL without credentials or fragment")
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &Publisher{endpoint: parsed, client: client}, nil
}

// Publish sends one decision as JSON. The run ID is carried as the
// idempotency key, so retrying this call does not create a new Nublar run.
func (p *Publisher) Publish(ctx context.Context, decision delivery.Decision) error {
	if p == nil || p.endpoint == nil || p.client == nil {
		return fmt.Errorf("Nublar webhook publisher is not configured")
	}
	var body bytes.Buffer
	if err := delivery.WriteJSON(&body, decision); err != nil {
		return fmt.Errorf("encode Nublar webhook decision: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint.String(), &body)
	if err != nil {
		return fmt.Errorf("create Nublar webhook request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", decision.RunID)
	response, err := p.client.Do(request)
	if err != nil {
		return fmt.Errorf("publish Nublar webhook decision: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Nublar webhook returned HTTP %s", response.Status)
	}
	return nil
}
