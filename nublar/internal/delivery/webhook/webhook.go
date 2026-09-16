// Package webhook publishes Nublar decisions to a generic HTTP endpoint.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ingen/nublar/internal/delivery"
)

type Publisher struct {
	endpoint *url.URL
	client   *http.Client
	secret   []byte
}

var _ delivery.Publisher = (*Publisher)(nil)

// New creates a generic HTTP webhook publisher. Authentication and retry
// policy are intentionally configured by a future concrete adapter.
func New(endpoint string, client *http.Client) (*Publisher, error) {
	return NewWithSecret(endpoint, client, nil)
}

// NewWithSecret creates a webhook publisher that optionally signs request
// bodies with HMAC-SHA256. The secret is copied before returning.
func NewWithSecret(endpoint string, client *http.Client, secret []byte) (*Publisher, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return nil, fmt.Errorf("parse Nublar webhook endpoint: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, fmt.Errorf("Nublar webhook endpoint must be an absolute HTTP(S) URL without credentials or fragment")
	}
	if client == nil {
		client = &http.Client{CheckRedirect: func(request *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}}
	}
	return &Publisher{endpoint: parsed, client: client, secret: append([]byte(nil), secret...)}, nil
}

// Publish sends one decision as JSON. The run ID is carried as the
// idempotency key, so retrying this call does not create a new Nublar run.
func (p *Publisher) Publish(ctx context.Context, decision delivery.Decision) (delivery.Receipt, error) {
	if p == nil || p.endpoint == nil || p.client == nil {
		return delivery.Receipt{}, fmt.Errorf("Nublar webhook publisher is not configured")
	}
	if err := decision.Validate(); err != nil {
		return delivery.Receipt{}, err
	}
	receipt := delivery.Receipt{
		Schema:      delivery.ReceiptSchema,
		RunID:       decision.RunID,
		Transport:   "http-webhook",
		Status:      "failed",
		AttemptedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	var body bytes.Buffer
	if err := delivery.WriteJSON(&body, decision); err != nil {
		receipt.Error = fmt.Sprintf("encode Nublar webhook decision: %v", err)
		return receipt, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint.String(), &body)
	if err != nil {
		receipt.Error = fmt.Sprintf("create Nublar webhook request: %v", err)
		return receipt, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", decision.RunID)
	if len(p.secret) > 0 {
		hasher := hmac.New(sha256.New, p.secret)
		_, _ = hasher.Write(body.Bytes())
		request.Header.Set("X-InGen-Signature-256", "sha256="+hex.EncodeToString(hasher.Sum(nil)))
	}
	response, err := p.client.Do(request)
	if err != nil {
		receipt.Error = fmt.Sprintf("publish Nublar webhook decision: %v", err)
		return receipt, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	receipt.HTTPStatus = response.StatusCode
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		receipt.Error = fmt.Sprintf("Nublar webhook returned HTTP %s", response.Status)
		return receipt, fmt.Errorf("%s", receipt.Error)
	}
	receipt.Status = "accepted"
	return receipt, nil
}
