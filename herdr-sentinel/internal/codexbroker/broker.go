// Package codexbroker exposes one short-lived, request-bounded Responses API
// endpoint for a single Codex execution. It keeps the upstream API key in the
// broker process and authenticates loopback clients with a scoped random token.
package codexbroker

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultMaxRequests     = 16
	DefaultMaxOutputTokens = 4096
	DefaultTimeout         = 300 * time.Second
	MaxMaxRequests         = 64
	MaxOutputTokens        = 8192
	MaxTimeout             = 1800 * time.Second
	MaxRequestBytes        = 8 << 20
	MaxResponseBytes       = 32 << 20
	closeWaitTimeout       = 5 * time.Second
	upstreamResponsesURL   = "https://api.openai.com/v1/responses"
)

var (
	modelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$`)
	toolPattern  = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

// Config is trusted broker configuration. Transport is a programmatic test
// seam; the CLI must not expose arbitrary upstream URLs or transports.
type Config struct {
	Port            int
	APIKey          string
	Model           string
	MaxRequests     int
	MaxOutputTokens int
	Timeout         time.Duration
	Transport       http.RoundTripper
}

// Stats contains counters and timestamps only; request content, headers,
// tokens, and credentials are never retained here.
type Stats struct {
	StartedAt         time.Time  `json:"started_at"`
	ClosedAt          *time.Time `json:"closed_at,omitempty"`
	RequestsReceived  int        `json:"requests_received"`
	RequestsForwarded int        `json:"requests_forwarded"`
	RequestsRejected  int        `json:"requests_rejected"`
	UpstreamFailures  int        `json:"upstream_failures"`
	InFlight          int        `json:"in_flight"`
	LastStatus        int        `json:"last_status"`
	LastCompletedAt   *time.Time `json:"last_completed_at,omitempty"`
}

// Server owns a loopback listener and one opaque bearer token. Close is
// idempotent and stops all active requests.
type Server struct {
	listener   net.Listener
	httpServer *http.Server
	client     *http.Client
	model      string
	maxReq     int
	maxTokens  int
	timeout    time.Duration
	endpoint   string
	lifetime   context.Context
	cancelLife context.CancelFunc

	mu        sync.Mutex
	token     string
	apiKey    string
	stats     Stats
	closed    bool
	forwardWG sync.WaitGroup
	handlerWG sync.WaitGroup
	closeOnce sync.Once
	closeErr  error
	closeDone chan struct{}
}

// Start binds only to IPv4 loopback. Port zero requests an ephemeral port for
// trusted callers; production role policies should pin a selected fixed port.
func Start(ctx context.Context, cfg Config) (*Server, error) {
	if ctx == nil {
		return nil, errors.New("broker context is required")
	}
	if ctx.Err() != nil {
		return nil, fmt.Errorf("broker context is already done: %w", ctx.Err())
	}
	if cfg.Port < 0 || cfg.Port > 65535 {
		return nil, errors.New("broker port must be between 0 and 65535")
	}
	if strings.TrimSpace(cfg.APIKey) == "" || strings.ContainsAny(cfg.APIKey, "\r\n\x00") || len(cfg.APIKey) > 4096 {
		return nil, errors.New("broker API credential is required and must be a bounded single-line value")
	}
	if !modelPattern.MatchString(cfg.Model) {
		return nil, errors.New("broker model must be a bounded safe model token")
	}
	if cfg.MaxRequests == 0 {
		cfg.MaxRequests = DefaultMaxRequests
	}
	if cfg.MaxRequests < 1 || cfg.MaxRequests > MaxMaxRequests {
		return nil, fmt.Errorf("broker max requests must be between 1 and %d", MaxMaxRequests)
	}
	if cfg.MaxOutputTokens == 0 {
		cfg.MaxOutputTokens = DefaultMaxOutputTokens
	}
	if cfg.MaxOutputTokens < 1 || cfg.MaxOutputTokens > MaxOutputTokens {
		return nil, fmt.Errorf("broker max output tokens must be between 1 and %d", MaxOutputTokens)
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.Timeout < time.Second || cfg.Timeout > MaxTimeout {
		return nil, fmt.Errorf("broker timeout must be between one second and %s", MaxTimeout)
	}

	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.Port)))
	if err != nil {
		return nil, fmt.Errorf("bind Responses broker to loopback: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("create scoped broker token: %w", err)
	}
	transport := cfg.Transport
	if transport == nil {
		transport = &http.Transport{
			Proxy:               nil,
			TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
			ForceAttemptHTTP2:   true,
			MaxIdleConns:        2,
			MaxIdleConnsPerHost: 1,
			IdleConnTimeout:     30 * time.Second,
		}
	}
	lifetime, cancelLife := context.WithTimeout(ctx, cfg.Timeout)
	server := &Server{
		listener:   listener,
		model:      cfg.Model,
		maxReq:     cfg.MaxRequests,
		maxTokens:  cfg.MaxOutputTokens,
		timeout:    cfg.Timeout,
		endpoint:   "http://127.0.0.1:" + strconv.Itoa(port) + "/v1",
		lifetime:   lifetime,
		cancelLife: cancelLife,
		closeDone:  make(chan struct{}),
		token:      base64.RawURLEncoding.EncodeToString(tokenBytes),
		apiKey:     cfg.APIKey,
		client: &http.Client{
			Transport:     transport,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		stats: Stats{StartedAt: time.Now().UTC()},
	}
	server.httpServer = &http.Server{
		Handler:           server,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.Timeout,
		WriteTimeout:      cfg.Timeout,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
	go func() {
		_ = server.httpServer.Serve(listener)
	}()
	go func() {
		<-lifetime.Done()
		_ = server.Close()
	}()
	return server, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		s.writeError(w, http.StatusServiceUnavailable, "broker is closed")
		return
	}
	s.handlerWG.Add(1)
	s.stats.RequestsReceived++
	s.mu.Unlock()
	defer s.handlerWG.Done()
	if err := s.validateTarget(r); err != nil {
		s.reject(w, http.StatusNotFound, "endpoint not available")
		return
	}
	if r.Method != http.MethodPost {
		s.reject(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.authorized(r.Header) {
		s.reject(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := validateContentType(r.Header); err != nil {
		s.reject(w, http.StatusUnsupportedMediaType, "content type must be application/json")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxRequestBytes+1))
	if err != nil {
		s.reject(w, http.StatusBadRequest, "request body could not be read")
		return
	}
	if len(body) > MaxRequestBytes {
		s.reject(w, http.StatusRequestEntityTooLarge, "request body exceeds broker limit")
		return
	}
	forwardBody, err := s.validateAndConstrainBody(body)
	if err != nil {
		s.reject(w, http.StatusBadRequest, err.Error())
		return
	}
	accept := selectAccept(r.Header)
	key, ok := s.beginForward()
	if !ok {
		s.reject(w, http.StatusTooManyRequests, "broker request budget or in-flight limit reached")
		return
	}
	defer s.finishForward()
	requestCtx, requestCancel := context.WithCancel(r.Context())
	stopLifetime := context.AfterFunc(s.lifetime, requestCancel)
	defer stopLifetime()
	ctx, cancel := context.WithTimeout(requestCtx, s.timeout)
	defer cancel()
	defer requestCancel()
	upstream, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamResponsesURL, strings.NewReader(string(forwardBody)))
	if err != nil {
		s.upstreamFailure()
		s.writeError(w, http.StatusBadGateway, "upstream request could not be prepared")
		return
	}
	upstream.Header.Set("Content-Type", "application/json")
	upstream.Header.Set("Accept", accept)
	upstream.Header.Set("Authorization", "Bearer "+key)
	upstream.Header.Set("User-Agent", "ingen-codexbroker/1")
	if values := r.Header.Values("OpenAI-Beta"); len(values) == 1 && values[0] == "responses=experimental" {
		upstream.Header.Set("OpenAI-Beta", values[0])
	}
	resp, err := s.client.Do(upstream)
	if err != nil {
		s.upstreamFailure()
		if r.Context().Err() == nil {
			s.writeError(w, http.StatusBadGateway, "upstream request failed")
		}
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 100 || resp.StatusCode > 599 {
		s.upstreamFailure()
		s.writeError(w, http.StatusBadGateway, "upstream returned an invalid response")
		return
	}
	s.recordUpstreamStatus(resp.StatusCode)
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		s.upstreamFailure()
		s.writeError(w, http.StatusBadGateway, "upstream redirects are not allowed")
		return
	}
	if resp.StatusCode >= 400 {
		s.upstreamFailure()
		s.writeError(w, resp.StatusCode, "upstream rejected the Responses API request")
		return
	}
	if resp.ContentLength > MaxResponseBytes {
		s.upstreamFailure()
		s.writeError(w, http.StatusBadGateway, "upstream response exceeds broker limit")
		return
	}
	copyResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	if err := copyBounded(w, resp.Body, MaxResponseBytes, resp.Header.Get("Content-Type")); err != nil {
		s.upstreamFailure()
		return
	}
}

func (s *Server) validateTarget(r *http.Request) error {
	if r.URL == nil || r.URL.IsAbs() || r.URL.Opaque != "" || r.URL.User != nil ||
		r.URL.EscapedPath() != "/v1/responses" || r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery || r.RequestURI != "/v1/responses" {
		return errors.New("invalid request target")
	}
	parsed, err := url.Parse(s.endpoint)
	if err != nil || r.Host != parsed.Host {
		return errors.New("invalid host")
	}
	return nil
}

func (s *Server) authorized(headers http.Header) bool {
	values := headers.Values("Authorization")
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
		return false
	}
	provided := strings.TrimPrefix(values[0], "Bearer ")
	s.mu.Lock()
	token := s.token
	open := !s.closed
	s.mu.Unlock()
	providedHash := sha256.Sum256([]byte(provided))
	tokenHash := sha256.Sum256([]byte(token))
	return open && subtle.ConstantTimeCompare(providedHash[:], tokenHash[:]) == 1
}

func validateContentType(headers http.Header) error {
	values := headers.Values("Content-Type")
	if len(values) != 1 {
		return errors.New("content type missing or duplicated")
	}
	mediaType, _, err := mime.ParseMediaType(values[0])
	if err != nil || !strings.EqualFold(mediaType, "application/json") {
		return errors.New("unsupported content type")
	}
	return nil
}

func selectAccept(headers http.Header) string {
	values := headers.Values("Accept")
	if len(values) != 1 {
		return "application/json"
	}
	switch strings.TrimSpace(values[0]) {
	case "text/event-stream", "application/json", "*/*":
		return strings.TrimSpace(values[0])
	default:
		return "application/json"
	}
}

func (s *Server) validateAndConstrainBody(data []byte) ([]byte, error) {
	if !json.Valid(data) {
		return nil, errors.New("request body must be valid JSON")
	}
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return nil, errors.New("request body contains duplicate JSON keys")
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(data, &body); err != nil || body == nil {
		return nil, errors.New("request body must be a JSON object")
	}
	allowed := map[string]bool{
		"model": true, "input": true, "instructions": true, "tools": true,
		"tool_choice": true, "parallel_tool_calls": true, "max_output_tokens": true,
		"stream": true, "store": true, "background": true, "text": true,
		"reasoning": true, "temperature": true, "top_p": true,
		"include": true, "metadata": true, "truncation": true,
		"client_metadata": true, "prompt_cache_key": true,
	}
	for key := range body {
		if !allowed[key] {
			return nil, errors.New("unsupported Responses API request field")
		}
	}
	if raw, ok := body["model"]; ok {
		var model string
		if err := json.Unmarshal(raw, &model); err != nil || model != s.model {
			return nil, errors.New("request model does not match the broker-selected model")
		}
	}
	body["model"], _ = json.Marshal(s.model)
	input, ok := body["input"]
	if !ok {
		return nil, errors.New("request input is required")
	}
	var inputValue any
	if err := json.Unmarshal(input, &inputValue); err != nil {
		return nil, errors.New("request input is invalid")
	}
	if text, isString := inputValue.(string); isString {
		if len(text) > MaxRequestBytes/2 {
			return nil, errors.New("request input exceeds broker limits")
		}
	} else if items, isArray := inputValue.([]any); isArray {
		if len(items) > 4096 {
			return nil, errors.New("request input has too many items")
		}
		for _, item := range items {
			if err := validateInputItem(item); err != nil {
				return nil, err
			}
		}
	} else {
		return nil, errors.New("request input must be a string or array")
	}
	if raw, ok := body["instructions"]; ok {
		var instructions string
		if err := json.Unmarshal(raw, &instructions); err != nil {
			return nil, errors.New("instructions must be a string")
		}
	}
	if raw, ok := body["background"]; ok {
		var background bool
		if err := json.Unmarshal(raw, &background); err != nil || background {
			return nil, errors.New("background responses are not supported")
		}
		delete(body, "background")
	}
	if raw, ok := body["store"]; ok {
		var store bool
		if err := json.Unmarshal(raw, &store); err != nil {
			return nil, errors.New("store must be a boolean")
		}
	}
	body["store"] = json.RawMessage("false")
	if raw, ok := body["max_output_tokens"]; ok {
		var requested json.Number
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		if err := decoder.Decode(&requested); err != nil {
			return nil, errors.New("max_output_tokens must be a positive integer")
		}
		count, err := strconv.Atoi(requested.String())
		if err != nil || count < 1 {
			return nil, errors.New("max_output_tokens must be a positive integer")
		}
		if count > s.maxTokens {
			count = s.maxTokens
		}
		body["max_output_tokens"], _ = json.Marshal(count)
	} else {
		body["max_output_tokens"], _ = json.Marshal(s.maxTokens)
	}
	if raw, ok := body["stream"]; ok {
		var stream bool
		if err := json.Unmarshal(raw, &stream); err != nil {
			return nil, errors.New("stream must be a boolean")
		}
	}
	if raw, ok := body["parallel_tool_calls"]; ok {
		var parallel bool
		if err := json.Unmarshal(raw, &parallel); err != nil {
			return nil, errors.New("parallel_tool_calls must be a boolean")
		}
	}
	if raw, ok := body["tools"]; ok {
		if err := validateTools(raw); err != nil {
			return nil, err
		}
	}
	if raw, ok := body["tool_choice"]; ok {
		if err := validateToolChoice(raw); err != nil {
			return nil, err
		}
	}
	if raw, ok := body["include"]; ok {
		if len(raw) == 0 || raw[0] != '[' {
			return nil, errors.New("include must be a string array")
		}
		var values []string
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, errors.New("include must be a string array")
		}
		for _, value := range values {
			if value != "reasoning.encrypted_content" {
				return nil, errors.New("requested include value is not permitted")
			}
		}
	}
	if raw, ok := body["client_metadata"]; ok {
		var metadata map[string]json.RawMessage
		if len(raw) > 64<<10 || json.Unmarshal(raw, &metadata) != nil || metadata == nil || len(metadata) > 64 {
			return nil, errors.New("client_metadata must be a bounded JSON object")
		}
		for key := range metadata {
			if len(key) == 0 || len(key) > 128 {
				return nil, errors.New("client_metadata keys exceed broker field limits")
			}
		}
		// Client metadata can carry host/session identifiers. It is useful to
		// Codex locally, but is not part of the upstream request contract here.
		delete(body, "client_metadata")
	}
	if raw, ok := body["prompt_cache_key"]; ok {
		var cacheKey string
		if err := json.Unmarshal(raw, &cacheKey); err != nil || len(cacheKey) == 0 || len(cacheKey) > 128 || strings.ContainsAny(cacheKey, "\r\n\x00") {
			return nil, errors.New("prompt_cache_key must be a bounded string")
		}
		delete(body, "prompt_cache_key")
	}
	if raw, ok := body["metadata"]; ok {
		var metadata map[string]string
		if err := json.Unmarshal(raw, &metadata); err != nil || metadata == nil || len(metadata) > 16 {
			return nil, errors.New("metadata must be a bounded string map")
		}
		for key, value := range metadata {
			if len(key) > 64 || len(value) > 256 {
				return nil, errors.New("metadata exceeds broker field limits")
			}
		}
		delete(body, "metadata")
	}
	if raw, ok := body["truncation"]; ok {
		var truncation string
		if err := json.Unmarshal(raw, &truncation); err != nil || (truncation != "auto" && truncation != "disabled") {
			return nil, errors.New("truncation must be auto or disabled")
		}
	}
	for _, field := range []string{"text", "reasoning"} {
		if raw, ok := body[field]; ok {
			var value map[string]json.RawMessage
			if err := json.Unmarshal(raw, &value); err != nil || value == nil {
				return nil, fmt.Errorf("%s must be an object", field)
			}
		}
	}
	for _, field := range []string{"temperature", "top_p"} {
		if raw, ok := body[field]; ok {
			var value float64
			max := 2.0
			if field == "top_p" {
				max = 1
			}
			if err := json.Unmarshal(raw, &value); err != nil || value < 0 || value > max {
				return nil, fmt.Errorf("%s must be a bounded number", field)
			}
		}
	}
	constrained, err := json.Marshal(body)
	if err != nil {
		return nil, errors.New("request could not be constrained")
	}
	return constrained, nil
}

// validateInputItem allows only self-contained text, reasoning, and local
// function-call exchange items. Stored provider items, file references,
// conversations, images, and remote URLs are rejected before any forwarding.
func validateInputItem(value any) error {
	item, ok := value.(map[string]any)
	if !ok || len(item) == 0 {
		return errors.New("request input items must be supported objects")
	}
	getString := func(key string, required bool, max int) (string, bool) {
		v, exists := item[key]
		if !exists {
			return "", !required
		}
		text, valid := v.(string)
		return text, valid && len(text) <= max && !strings.ContainsRune(text, '\x00')
	}
	typeName, valid := getString("type", true, 64)
	if !valid {
		return errors.New("request input item type is invalid")
	}
	switch typeName {
	case "message":
		if !onlyKeys(item, "type", "role", "content", "id", "status") {
			return errors.New("message input contains unsupported fields")
		}
		if !optionalBoundedStringOrNull(item, "id", 256) {
			return errors.New("message input id is invalid")
		}
		if status, ok := getString("status", false, 32); !ok || status != "" && status != "in_progress" && status != "completed" {
			return errors.New("message input status is invalid")
		}
		role, ok := getString("role", true, 32)
		if !ok || (role != "user" && role != "assistant" && role != "developer" && role != "system") {
			return errors.New("message input role is unsupported")
		}
		content, ok := item["content"]
		if !ok {
			return errors.New("message input content is required")
		}
		if text, ok := content.(string); ok {
			if len(text) > MaxRequestBytes/2 {
				return errors.New("message text exceeds broker limits")
			}
			return nil
		}
		parts, ok := content.([]any)
		if !ok || len(parts) > 4096 {
			return errors.New("message content must contain text parts")
		}
		for _, partValue := range parts {
			part, ok := partValue.(map[string]any)
			if !ok || !onlyKeys(part, "type", "text") {
				return errors.New("message content contains unsupported parts")
			}
			partType, typeOK := part["type"].(string)
			text, textOK := part["text"].(string)
			if !typeOK || partType != "input_text" && partType != "output_text" || !textOK || len(text) > MaxRequestBytes/2 {
				return errors.New("message content must be plain text")
			}
		}
		return nil
	case "function_call":
		if !onlyKeys(item, "type", "call_id", "name", "arguments", "id", "status") {
			return errors.New("function call contains unsupported fields")
		}
		callID, callOK := getString("call_id", true, 128)
		name, nameOK := getString("name", true, 64)
		_, argsOK := getString("arguments", true, MaxRequestBytes/2)
		if !callOK || callID == "" || !nameOK || !toolPattern.MatchString(name) || !argsOK {
			return errors.New("function call fields are invalid")
		}
		return nil
	case "function_call_output":
		if !onlyKeys(item, "type", "call_id", "output") {
			return errors.New("function call output contains unsupported fields")
		}
		callID, callOK := getString("call_id", true, 128)
		_, outputOK := getString("output", true, MaxRequestBytes/2)
		if !callOK || callID == "" || !outputOK {
			return errors.New("function call output fields are invalid")
		}
		return nil
	case "custom_tool_call":
		if !onlyKeys(item, "type", "call_id", "name", "input", "id", "status") {
			return errors.New("custom tool call contains unsupported fields")
		}
		callID, callOK := getString("call_id", true, 128)
		name, nameOK := getString("name", true, 64)
		_, inputOK := getString("input", true, MaxRequestBytes/2)
		if !callOK || callID == "" || !nameOK || !toolPattern.MatchString(name) || !inputOK {
			return errors.New("custom tool call fields are invalid")
		}
		if !optionalBoundedStringOrNull(item, "id", 256) {
			return errors.New("custom tool call id is invalid")
		}
		if status, ok := getString("status", false, 32); !ok || status != "" && status != "in_progress" && status != "completed" {
			return errors.New("custom tool call status is invalid")
		}
		return nil
	case "custom_tool_call_output":
		if !onlyKeys(item, "type", "call_id", "output") {
			return errors.New("custom tool output contains unsupported fields")
		}
		callID, callOK := getString("call_id", true, 128)
		_, outputOK := getString("output", true, MaxRequestBytes/2)
		if !callOK || callID == "" || !outputOK {
			return errors.New("custom tool output fields are invalid")
		}
		return nil
	case "reasoning":
		if !onlyKeys(item, "type", "id", "summary", "encrypted_content", "status") {
			return errors.New("reasoning item contains unsupported fields")
		}
		if _, ok := getString("id", false, 256); !ok {
			return errors.New("reasoning item id is invalid")
		}
		if _, ok := getString("encrypted_content", false, MaxRequestBytes/2); !ok {
			return errors.New("reasoning item content is invalid")
		}
		if summaryValue, exists := item["summary"]; exists {
			summary, ok := summaryValue.([]any)
			if !ok || len(summary) > 128 {
				return errors.New("reasoning summary is invalid")
			}
			for _, value := range summary {
				part, ok := value.(map[string]any)
				if !ok || !onlyKeys(part, "type", "text") {
					return errors.New("reasoning summary contains unsupported fields")
				}
				kind, kindOK := part["type"].(string)
				text, textOK := part["text"].(string)
				if !kindOK || kind != "summary_text" || !textOK || len(text) > MaxRequestBytes/2 {
					return errors.New("reasoning summary must contain bounded text")
				}
			}
		}
		return nil
	default:
		return errors.New("request input item type is not permitted")
	}
}

func onlyKeys(object map[string]any, keys ...string) bool {
	allowed := make(map[string]bool, len(keys))
	for _, key := range keys {
		allowed[key] = true
	}
	for key := range object {
		if !allowed[key] {
			return false
		}
	}
	return true
}

func optionalBoundedStringOrNull(object map[string]any, key string, max int) bool {
	value, exists := object[key]
	if !exists || value == nil {
		return true
	}
	text, ok := value.(string)
	return ok && len(text) <= max && !strings.ContainsRune(text, '\x00')
}

func validateTools(raw json.RawMessage) error {
	if len(raw) == 0 || raw[0] != '[' {
		return errors.New("tools must be an array of local function or custom tools")
	}
	var tools []json.RawMessage
	if err := json.Unmarshal(raw, &tools); err != nil {
		return errors.New("tools must be an array of local function or custom tools")
	}
	seen := map[string]bool{}
	for _, rawTool := range tools {
		var tool map[string]json.RawMessage
		if err := json.Unmarshal(rawTool, &tool); err != nil || tool == nil {
			return errors.New("tool declarations must be objects")
		}
		var kind, name string
		if json.Unmarshal(tool["type"], &kind) != nil || json.Unmarshal(tool["name"], &name) != nil || !toolPattern.MatchString(name) {
			return errors.New("tools must name a supported local function or custom tool")
		}
		if seen[name] {
			return errors.New("tool names must be unique")
		}
		seen[name] = true
		allowed := map[string]bool{"type": true, "name": true, "description": true}
		switch kind {
		case "function":
			allowed["parameters"] = true
			allowed["strict"] = true
		case "custom":
			allowed["format"] = true
		default:
			return errors.New("provider-hosted tools are not supported")
		}
		for key := range tool {
			if !allowed[key] {
				return errors.New("tool contains an unsupported field")
			}
		}
		if description, ok := tool["description"]; ok {
			var text string
			if json.Unmarshal(description, &text) != nil || len(text) > 2048 {
				return errors.New("tool description must be a bounded string")
			}
		}
		if kind == "function" {
			if parameters, ok := tool["parameters"]; ok && !json.Valid(parameters) {
				return errors.New("function parameters must be valid JSON Schema")
			}
			if strict, ok := tool["strict"]; ok {
				var value bool
				if json.Unmarshal(strict, &value) != nil {
					return errors.New("function strict flag must be boolean")
				}
			}
		}
	}
	return nil
}

func validateToolChoice(raw json.RawMessage) error {
	var choice any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&choice); err != nil {
		return errors.New("tool_choice is invalid")
	}
	if text, ok := choice.(string); ok {
		if text == "auto" || text == "none" || text == "required" {
			return nil
		}
		return errors.New("tool_choice must select a local function or custom tool")
	}
	object, ok := choice.(map[string]any)
	if !ok || len(object) != 2 {
		return errors.New("tool_choice must select a local function or custom tool")
	}
	kind, kindOK := object["type"].(string)
	name, nameOK := object["name"].(string)
	if !kindOK || !nameOK || (kind != "function" && kind != "custom") || !toolPattern.MatchString(name) {
		return errors.New("tool_choice must select a local function or custom tool")
	}
	return nil
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := scanJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("multiple JSON values")
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok || seen[key] {
				return errors.New("duplicate or invalid object key")
			}
			seen[key] = true
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("unterminated JSON object")
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("unterminated JSON array")
		}
	default:
		return errors.New("unexpected JSON delimiter")
	}
	return nil
}

func (s *Server) beginForward() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.stats.InFlight != 0 || s.stats.RequestsForwarded >= s.maxReq {
		return "", false
	}
	s.stats.RequestsForwarded++
	s.stats.InFlight++
	s.forwardWG.Add(1)
	return s.apiKey, true
}

func (s *Server) finishForward() {
	s.mu.Lock()
	if s.stats.InFlight > 0 {
		s.stats.InFlight--
	}
	now := time.Now().UTC()
	s.stats.LastCompletedAt = &now
	s.mu.Unlock()
	s.forwardWG.Done()
}

func (s *Server) upstreamFailure() {
	s.mu.Lock()
	s.stats.UpstreamFailures++
	s.mu.Unlock()
}

func (s *Server) recordUpstreamStatus(status int) {
	s.mu.Lock()
	s.stats.LastStatus = status
	s.mu.Unlock()
}

func (s *Server) reject(w http.ResponseWriter, status int, message string) {
	s.mu.Lock()
	s.stats.RequestsRejected++
	s.mu.Unlock()
	s.writeError(w, status, message)
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": message}})
}

func copyResponseHeaders(dst, src http.Header) {
	for _, name := range []string{"Content-Type", "Cache-Control"} {
		values := src.Values(name)
		if len(values) == 1 && len(values[0]) <= 512 && !strings.ContainsAny(values[0], "\r\n") {
			dst.Set(name, values[0])
		}
	}
	if dst.Get("Content-Type") == "" {
		dst.Set("Content-Type", "application/json")
	}
}

func copyBounded(dst http.ResponseWriter, src io.Reader, limit int64, contentType string) error {
	buffer := make([]byte, 32<<10)
	var written int64
	flusher, canFlush := dst.(http.Flusher)
	for {
		read, readErr := src.Read(buffer)
		if read > 0 {
			remaining := limit - written
			toWrite := int64(read)
			if toWrite > remaining {
				toWrite = remaining
			}
			if toWrite > 0 {
				n, writeErr := dst.Write(buffer[:int(toWrite)])
				written += int64(n)
				if writeErr != nil {
					return writeErr
				}
				if canFlush && strings.HasPrefix(contentType, "text/event-stream") {
					flusher.Flush()
				}
			}
			if int64(read) > remaining {
				return errors.New("upstream response body exceeds broker limit")
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
		if read == 0 {
			return nil
		}
	}
}

// Token returns the random per-server bearer credential for the local client.
// It is scoped to this listener and should only be placed in the child agent's
// designated broker-token environment variable.
func (s *Server) Token() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token
}

// Endpoint returns the local base URL ending in /v1.
func (s *Server) Endpoint() string { return s.endpoint }

// Snapshot returns secret-free counters and lifecycle timestamps.
func (s *Server) Snapshot() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := s.stats
	if copy.ClosedAt != nil {
		value := *copy.ClosedAt
		copy.ClosedAt = &value
	}
	if copy.LastCompletedAt != nil {
		value := *copy.LastCompletedAt
		copy.LastCompletedAt = &value
	}
	return copy
}

// Close immediately terminates active HTTP requests and invalidates the token.
func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.token = ""
		s.apiKey = ""
		s.cancelLife()
		s.mu.Unlock()
		s.closeErr = s.httpServer.Close()
		if errors.Is(s.closeErr, http.ErrServerClosed) || errors.Is(s.closeErr, net.ErrClosed) {
			s.closeErr = nil
		}
		if closer, ok := s.client.Transport.(interface{ CloseIdleConnections() }); ok {
			closer.CloseIdleConnections()
		}
		go func() {
			s.handlerWG.Wait()
			s.forwardWG.Wait()
			s.mu.Lock()
			now := time.Now().UTC()
			s.stats.ClosedAt = &now
			s.mu.Unlock()
			close(s.closeDone)
		}()
	})
	select {
	case <-s.closeDone:
		return s.closeErr
	case <-time.After(closeWaitTimeout):
		return errors.New("broker shutdown did not settle active upstream work before the close deadline")
	}
}
