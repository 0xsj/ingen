package codexbroker

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

const testCredential = "broker-test-credential-never-real"

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func testConfig(transport http.RoundTripper) Config {
	return Config{Port: 0, APIKey: testCredential, Model: "test-codex-model", MaxRequests: 2, MaxOutputTokens: 128, Timeout: 3 * time.Second, Transport: transport}
}

func startTestBroker(t *testing.T, config Config) *Server {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	server, err := Start(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	return server
}

func requestBroker(t *testing.T, server *Server, body string, mutate func(*http.Request)) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, server.Endpoint()+"/responses", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+server.Token())
	request.Header.Set("Content-Type", "application/json")
	if mutate != nil {
		mutate(request)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func readResponse(t *testing.T, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestStartValidatesBoundedTrustedConfiguration(t *testing.T) {
	for name, mutate := range map[string]func(*Config){
		"port":                 func(config *Config) { config.Port = 65536 },
		"missing credential":   func(config *Config) { config.APIKey = "" },
		"multiline credential": func(config *Config) { config.APIKey = "key\nvalue" },
		"unsafe model":         func(config *Config) { config.Model = "model\r\nAuthorization: bad" },
		"too many requests":    func(config *Config) { config.MaxRequests = MaxMaxRequests + 1 },
		"too many tokens":      func(config *Config) { config.MaxOutputTokens = MaxOutputTokens + 1 },
		"long timeout":         func(config *Config) { config.Timeout = MaxTimeout + time.Second },
	} {
		t.Run(name, func(t *testing.T) {
			config := testConfig(nil)
			mutate(&config)
			if server, err := Start(context.Background(), config); err == nil {
				_ = server.Close()
				t.Fatal("Start unexpectedly accepted invalid configuration")
			}
		})
	}
}

func TestStartPinsLoopbackAndGeneratesScopedToken(t *testing.T) {
	server := startTestBroker(t, testConfig(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusOK, `{"id":"resp_test"}`, "application/json"), nil
	})))
	if !strings.HasPrefix(server.Endpoint(), "http://127.0.0.1:") || !strings.HasSuffix(server.Endpoint(), "/v1") {
		t.Fatalf("Endpoint() = %q", server.Endpoint())
	}
	if token := server.Token(); token == "" || len(token) < 32 {
		t.Fatalf("Token() has unexpected length %d", len(token))
	}
	other := startTestBroker(t, testConfig(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusOK, `{}`, "application/json"), nil
	})))
	if server.Token() == other.Token() {
		t.Fatal("separate broker instances reused a token")
	}
	if server.Endpoint() == other.Endpoint() {
		t.Fatal("separate ephemeral listeners reused an endpoint")
	}
}

func TestRequestConstrainedAndOnlyExplicitHeadersForwarded(t *testing.T) {
	var gotBody map[string]json.RawMessage
	var gotHeader http.Header
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != upstreamResponsesURL || request.Host != "api.openai.com" {
			t.Errorf("upstream URL/Host = %s / %s", request.URL, request.Host)
		}
		if request.Header.Get("Authorization") != "Bearer "+testCredential {
			t.Errorf("upstream Authorization = %q", request.Header.Get("Authorization"))
		}
		gotHeader = request.Header.Clone()
		data, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read upstream body: %v", err)
		}
		if err := json.Unmarshal(data, &gotBody); err != nil {
			t.Errorf("decode upstream body: %v", err)
		}
		return response(http.StatusOK, `{"id":"resp_1"}`, "application/json"), nil
	})
	server := startTestBroker(t, testConfig(transport))
	body := `{"model":"test-codex-model","input":[{"type":"message","id":null,"role":"user","content":[{"type":"input_text","text":"hello"}]},{"type":"reasoning","encrypted_content":"opaque-state"}],"max_output_tokens":999,"store":true,"background":false,"include":["reasoning.encrypted_content"],"client_metadata":{"session_id":"private-session"},"prompt_cache_key":"private-cache-key","metadata":{"workspace":"private-workspace"},"tools":[{"type":"function","name":"local_fn","parameters":{"type":"object"}}]}`
	resp := requestBroker(t, server, body, func(request *http.Request) {
		request.Header.Set("Accept", "text/event-stream")
		request.Header.Set("OpenAI-Beta", "responses=experimental")
		request.Header.Set("OpenAI-Organization", "should-not-forward")
		request.Header.Set("Cookie", "secret-cookie")
		request.Header.Set("X-Private-Header", "secret-header")
		request.Header.Set("User-Agent", "caller-agent")
	})
	if resp.StatusCode != http.StatusOK || !strings.Contains(readResponse(t, resp), "resp_1") {
		t.Fatalf("response status/body = %d", resp.StatusCode)
	}
	var model string
	_ = json.Unmarshal(gotBody["model"], &model)
	var tokenCount int
	_ = json.Unmarshal(gotBody["max_output_tokens"], &tokenCount)
	var store bool
	_ = json.Unmarshal(gotBody["store"], &store)
	if model != "test-codex-model" || tokenCount != 128 || store {
		t.Fatalf("constrained body model/tokens/store = %q/%d/%v", model, tokenCount, store)
	}
	if _, exists := gotBody["background"]; exists {
		t.Fatal("background:false was forwarded")
	}
	for _, field := range []string{"client_metadata", "prompt_cache_key", "metadata"} {
		if _, exists := gotBody[field]; exists {
			t.Errorf("local-only field %s was forwarded", field)
		}
	}
	if gotHeader.Get("Accept") != "text/event-stream" || gotHeader.Get("OpenAI-Beta") != "responses=experimental" {
		t.Fatalf("explicit allowed metadata headers were lost: %#v", gotHeader)
	}
	for _, name := range []string{"OpenAI-Organization", "Cookie", "X-Private-Header"} {
		if gotHeader.Get(name) != "" {
			t.Errorf("caller header %s reached upstream", name)
		}
	}
	if gotHeader.Get("User-Agent") != "ingen-codexbroker/1" {
		t.Fatalf("upstream User-Agent = %q", gotHeader.Get("User-Agent"))
	}
	stats := server.Snapshot()
	if stats.RequestsReceived != 1 || stats.RequestsForwarded != 1 || stats.RequestsRejected != 0 || stats.InFlight != 0 || stats.LastStatus != 200 || stats.LastCompletedAt == nil {
		t.Fatalf("stats after request = %+v", stats)
	}
}

func TestOpaqueClientMetadataIsBoundedButNotForwarded(t *testing.T) {
	var forwarded map[string]json.RawMessage
	server := startTestBroker(t, testConfig(roundTripFunc(func(request *http.Request) (*http.Response, error) {
		data, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &forwarded); err != nil {
			return nil, err
		}
		return response(http.StatusOK, `{}`, "application/json"), nil
	})))
	metadata := map[string]any{
		"opaque_multiline": strings.Repeat("line one\nline two\n", 64),
		"opaque_nested":    map[string]any{"array": []any{1, true, map[string]any{"value": "uninterpreted"}}},
	}
	body, err := json.Marshal(map[string]any{"model": "test-codex-model", "input": "hello", "client_metadata": metadata})
	if err != nil {
		t.Fatal(err)
	}
	resp := requestBroker(t, server, string(body), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bounded opaque metadata status/body = %d/%q", resp.StatusCode, readResponse(t, resp))
	}
	_ = resp.Body.Close()
	if _, ok := forwarded["client_metadata"]; ok {
		t.Fatal("opaque client_metadata reached upstream")
	}
	oversized, err := json.Marshal(map[string]any{"model": "test-codex-model", "input": "hello", "client_metadata": map[string]any{"opaque": strings.Repeat("x", 64<<10)}})
	if err != nil {
		t.Fatal(err)
	}
	resp = requestBroker(t, server, string(oversized), nil)
	if resp.StatusCode != http.StatusBadRequest {
		text := readResponse(t, resp)
		t.Fatalf("oversized metadata status/body = %d/%q", resp.StatusCode, text)
	}
	_ = resp.Body.Close()
}

func TestInvalidRequestsAreRejectedWithoutUpstreamAttempt(t *testing.T) {
	var upstreamCalls int
	server := startTestBroker(t, testConfig(roundTripFunc(func(*http.Request) (*http.Response, error) {
		upstreamCalls++
		return response(http.StatusOK, `{}`, "application/json"), nil
	})))
	cases := []struct {
		name       string
		body       string
		mutate     func(*http.Request)
		statusCode int
	}{
		{"wrong token", `{"input":"hi"}`, func(request *http.Request) { request.Header.Set("Authorization", "Bearer wrong") }, http.StatusUnauthorized},
		{"duplicate auth", `{"input":"hi"}`, func(request *http.Request) { request.Header.Add("Authorization", "Bearer "+server.Token()) }, http.StatusUnauthorized},
		{"query", `{"input":"hi"}`, func(request *http.Request) { request.URL.RawQuery = "x=1" }, http.StatusNotFound},
		{"wrong host", `{"input":"hi"}`, func(request *http.Request) { request.Host = "localhost" }, http.StatusNotFound},
		{"model mismatch", `{"model":"other","input":"hi"}`, nil, http.StatusBadRequest},
		{"previous response", `{"input":"hi","previous_response_id":"resp_1"}`, nil, http.StatusBadRequest},
		{"background true", `{"input":"hi","background":true}`, nil, http.StatusBadRequest},
		{"hosted tool", `{"input":"hi","tools":[{"type":"web_search_preview","name":"web"}]}`, nil, http.StatusBadRequest},
		{"provider item reference", `{"input":[{"type":"item_reference","id":"resp_item_1"}]}`, nil, http.StatusBadRequest},
		{"remote input image", `{"input":[{"type":"message","role":"user","content":[{"type":"input_image","image_url":"https://example.invalid/x.png"}]}]}`, nil, http.StatusBadRequest},
		{"provider file reference", `{"input":[{"type":"message","role":"user","content":[{"type":"input_file","file_id":"file_123"}]}]}`, nil, http.StatusBadRequest},
		{"remote conversation", `{"input":"hi","conversation":"conv_123"}`, nil, http.StatusBadRequest},
		{"unsupported reasoning include", `{"input":"hi","include":["message.input_image.image_url"]}`, nil, http.StatusBadRequest},
		{"duplicate nested key", `{"input":"hi","metadata":{"x":"a","x":"b"}}`, nil, http.StatusBadRequest},
		{"extra JSON value", `{"input":"hi"} {"input":"again"}`, nil, http.StatusBadRequest},
		{"wrong media", `{"input":"hi"}`, func(request *http.Request) { request.Header.Set("Content-Type", "text/plain") }, http.StatusUnsupportedMediaType},
		{"method", `{"input":"hi"}`, func(request *http.Request) { request.Method = http.MethodGet }, http.StatusMethodNotAllowed},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			resp := requestBroker(t, server, test.body, test.mutate)
			if resp.StatusCode != test.statusCode {
				text := readResponse(t, resp)
				t.Fatalf("status/body = %d/%q, want %d", resp.StatusCode, text, test.statusCode)
			}
			_ = resp.Body.Close()
		})
	}
	stats := server.Snapshot()
	if upstreamCalls != 0 || stats.RequestsForwarded != 0 || stats.RequestsRejected != len(cases) {
		t.Fatalf("invalid requests reached upstream: calls=%d stats=%+v", upstreamCalls, stats)
	}
}

func TestBudgetAndNoQueueConcurrency(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var calls int
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		entered <- struct{}{}
		<-release
		return response(http.StatusOK, `{}`, "application/json"), nil
	})
	config := testConfig(transport)
	config.MaxRequests = 1
	server := startTestBroker(t, config)
	firstDone := make(chan *http.Response, 1)
	go func() { firstDone <- requestBroker(t, server, `{"input":"first"}`, nil) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first request did not reach upstream")
	}
	second := requestBroker(t, server, `{"input":"second"}`, nil)
	if second.StatusCode != http.StatusTooManyRequests {
		_ = readResponse(t, second)
		t.Fatalf("concurrent request status = %d, want no-queue rejection", second.StatusCode)
	}
	_ = second.Body.Close()
	close(release)
	first := <-firstDone
	_ = readResponse(t, first)
	third := requestBroker(t, server, `{"input":"third"}`, nil)
	if third.StatusCode != http.StatusTooManyRequests {
		_ = readResponse(t, third)
		t.Fatalf("request after budget status = %d", third.StatusCode)
	}
	_ = third.Body.Close()
	stats := server.Snapshot()
	if calls != 1 || stats.RequestsForwarded != 1 || stats.RequestsRejected != 2 || stats.InFlight != 0 {
		t.Fatalf("budget stats = calls:%d %+v", calls, stats)
	}
}

func TestUpstreamFailureRedirectAndResponseLimitAreGeneric(t *testing.T) {
	t.Run("credential-bearing transport error is redacted", func(t *testing.T) {
		server := startTestBroker(t, testConfig(roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New(testCredential + " leaked in transport error")
		})))
		resp := requestBroker(t, server, `{"input":"sensitive request text"}`, nil)
		text := readResponse(t, resp)
		if resp.StatusCode != http.StatusBadGateway || strings.Contains(text, testCredential) || strings.Contains(text, "sensitive request text") {
			t.Fatalf("transport failure was not generic: status=%d body=%q", resp.StatusCode, text)
		}
	})
	t.Run("redirect is rejected", func(t *testing.T) {
		server := startTestBroker(t, testConfig(roundTripFunc(func(*http.Request) (*http.Response, error) {
			resp := response(http.StatusFound, "", "application/json")
			resp.Header.Set("Location", "https://attacker.invalid/capture")
			return resp, nil
		})))
		resp := requestBroker(t, server, `{"input":"hi"}`, nil)
		text := readResponse(t, resp)
		if resp.StatusCode != http.StatusBadGateway || resp.Header.Get("Location") != "" || strings.Contains(text, "attacker.invalid") {
			t.Fatalf("redirect was exposed/followed: status=%d headers=%#v body=%q", resp.StatusCode, resp.Header, text)
		}
		if server.Snapshot().UpstreamFailures != 1 {
			t.Fatalf("redirect failure stats = %+v", server.Snapshot())
		}
	})
	t.Run("upstream error body is sanitized", func(t *testing.T) {
		server := startTestBroker(t, testConfig(roundTripFunc(func(*http.Request) (*http.Response, error) {
			return response(http.StatusUnauthorized, `{"error":"`+testCredential+`"}`, "application/json"), nil
		})))
		resp := requestBroker(t, server, `{"input":"hi"}`, nil)
		text := readResponse(t, resp)
		if resp.StatusCode != http.StatusUnauthorized || strings.Contains(text, testCredential) || !strings.Contains(text, "upstream rejected") {
			t.Fatalf("upstream error body was not sanitized: status=%d body=%q", resp.StatusCode, text)
		}
	})
	t.Run("declared oversized response is rejected", func(t *testing.T) {
		server := startTestBroker(t, testConfig(roundTripFunc(func(*http.Request) (*http.Response, error) {
			resp := response(http.StatusOK, "", "application/json")
			resp.ContentLength = MaxResponseBytes + 1
			return resp, nil
		})))
		resp := requestBroker(t, server, `{"input":"hi"}`, nil)
		text := readResponse(t, resp)
		if resp.StatusCode != http.StatusBadGateway || strings.Contains(text, testCredential) {
			t.Fatalf("oversized response status/body = %d/%q", resp.StatusCode, text)
		}
	})
}

func TestStreamingResponseAndCancellation(t *testing.T) {
	t.Run("SSE bytes pass through", func(t *testing.T) {
		const event = "event: response.completed\ndata: {\"type\":\"response.completed\"}\n\n"
		server := startTestBroker(t, testConfig(roundTripFunc(func(*http.Request) (*http.Response, error) {
			return response(http.StatusOK, event, "text/event-stream"), nil
		})))
		resp := requestBroker(t, server, `{"input":"hi","stream":true}`, func(request *http.Request) { request.Header.Set("Accept", "text/event-stream") })
		if resp.Header.Get("Content-Type") != "text/event-stream" || readResponse(t, resp) != event {
			t.Fatal("SSE response was not passed through byte-identically")
		}
	})
	t.Run("caller cancellation reaches upstream", func(t *testing.T) {
		entered := make(chan struct{})
		transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
			close(entered)
			<-request.Context().Done()
			return nil, request.Context().Err()
		})
		server := startTestBroker(t, testConfig(transport))
		ctx, cancel := context.WithCancel(context.Background())
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.Endpoint()+"/responses", strings.NewReader(`{"input":"hi"}`))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+server.Token())
		request.Header.Set("Content-Type", "application/json")
		done := make(chan struct{})
		go func() { _, _ = http.DefaultClient.Do(request); close(done) }()
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("request did not reach the upstream transport")
		}
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("client request did not end after cancellation")
		}
		deadline := time.Now().Add(time.Second)
		for server.Snapshot().InFlight != 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		stats := server.Snapshot()
		if stats.InFlight != 0 || stats.UpstreamFailures != 1 {
			t.Fatalf("canceled request stats = %+v", stats)
		}
	})
}

func TestSecretsNeverAppearInStatsAndCloseInvalidatesToken(t *testing.T) {
	server := startTestBroker(t, testConfig(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusOK, `{}`, "application/json"), nil
	})))
	token := server.Token()
	resp := requestBroker(t, server, `{"input":"hi","secret-body-sentinel":"test"}`, nil)
	_ = readResponse(t, resp)
	statsBytes, err := json.Marshal(server.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(statsBytes), testCredential) || strings.Contains(string(statsBytes), token) || strings.Contains(string(statsBytes), "secret-body-sentinel") {
		t.Fatalf("secret or request content found in stats: %s", statsBytes)
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	if server.Token() != "" {
		t.Fatal("Close did not invalidate local token")
	}
	stats := server.Snapshot()
	if stats.ClosedAt == nil || stats.InFlight != 0 {
		t.Fatalf("closed stats = %+v", stats)
	}
	if err := server.Close(); err != nil {
		t.Fatalf("idempotent Close() = %v", err)
	}
}

func TestDefaultTransportDisablesAmbientProxyAndValidatesTLS(t *testing.T) {
	server := startTestBroker(t, testConfig(nil))
	transport, ok := server.client.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil || transport.TLSClientConfig == nil || transport.TLSClientConfig.MinVersion < tls.VersionTLS12 {
		t.Fatalf("default transport does not enforce no-proxy/TLS requirements: %#v", server.client.Transport)
	}
}

func TestConcurrentSnapshotCloseIsRaceSafe(t *testing.T) {
	server := startTestBroker(t, testConfig(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusOK, `{}`, "application/json"), nil
	})))
	var wait sync.WaitGroup
	for index := 0; index < 12; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range 30 {
				_ = server.Snapshot()
				_ = server.Token()
			}
		}()
	}
	wait.Add(1)
	go func() {
		defer wait.Done()
		_ = server.Close()
	}()
	wait.Wait()
	if server.Snapshot().ClosedAt == nil {
		t.Fatal("concurrent Close did not record closure")
	}
}

func TestCloseCancelsAndSettlesActiveForwardBeforeClosedTimestamp(t *testing.T) {
	entered := make(chan struct{})
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		close(entered)
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	server := startTestBroker(t, testConfig(transport))
	done := make(chan struct{})
	go func() {
		request, err := http.NewRequest(http.MethodPost, server.Endpoint()+"/responses", strings.NewReader(`{"input":"hi"}`))
		if err != nil {
			close(done)
			return
		}
		request.Header.Set("Authorization", "Bearer "+server.Token())
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err == nil {
			_ = response.Body.Close()
		}
		close(done)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("request did not reach upstream")
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("client request remained active after Close")
	}
	stats := server.Snapshot()
	if stats.ClosedAt == nil || stats.InFlight != 0 || stats.LastCompletedAt == nil || stats.LastCompletedAt.After(*stats.ClosedAt) {
		t.Fatalf("Close recorded unstable stats: %+v", stats)
	}
	if stats.RequestsReceived != stats.RequestsForwarded+stats.RequestsRejected {
		t.Fatalf("closed request accounting is inconsistent: %+v", stats)
	}
}

func TestConfiguredTimeoutBoundsWholeBrokerLifetime(t *testing.T) {
	entered := make(chan struct{})
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		close(entered)
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	config := testConfig(transport)
	config.Timeout = time.Second
	server := startTestBroker(t, config)
	done := make(chan struct{})
	go func() {
		response, err := requestBrokerNoFatal(server)
		if err == nil {
			_ = response.Body.Close()
		}
		close(done)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("request did not reach upstream")
	}
	deadline := time.Now().Add(3 * time.Second)
	for server.Snapshot().ClosedAt == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	stats := server.Snapshot()
	if stats.ClosedAt == nil || stats.InFlight != 0 || stats.LastCompletedAt == nil || stats.LastCompletedAt.After(*stats.ClosedAt) {
		t.Fatalf("configured total timeout did not settle broker: %+v", stats)
	}
	address := strings.TrimSuffix(strings.TrimPrefix(server.Endpoint(), "http://"), "/v1")
	connection, err := net.DialTimeout("tcp", address, 250*time.Millisecond)
	if err == nil {
		_ = connection.Close()
		t.Fatal("listener still accepted connections after the configured broker lifetime")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out client request remained active")
	}
}

func TestCloseSettlesHandlerBlockedReadingRequestBody(t *testing.T) {
	server := startTestBroker(t, testConfig(roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("incomplete request body reached upstream")
		return response(http.StatusInternalServerError, `{}`, "application/json"), nil
	})))
	reader := &blockedBodyReader{started: make(chan struct{}), release: make(chan struct{})}
	request, err := http.NewRequest(http.MethodPost, server.Endpoint()+"/responses", reader)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+server.Token())
	request.Header.Set("Content-Type", "application/json")
	done := make(chan struct{})
	go func() {
		response, requestErr := http.DefaultClient.Do(request)
		if requestErr == nil {
			_ = response.Body.Close()
		}
		close(done)
	}()
	select {
	case <-reader.started:
	case <-time.After(time.Second):
		close(reader.release)
		t.Fatal("client did not begin the partial request")
	}
	deadline := time.Now().Add(time.Second)
	for server.Snapshot().RequestsReceived == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if server.Snapshot().RequestsReceived != 1 {
		close(reader.release)
		t.Fatal("broker did not admit the body-reading handler")
	}
	if err := server.Close(); err != nil {
		close(reader.release)
		t.Fatal(err)
	}
	close(reader.release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("client request remained active after Close")
	}
	stats := server.Snapshot()
	if stats.ClosedAt == nil || stats.RequestsReceived != 1 || stats.RequestsRejected != 1 || stats.RequestsForwarded != 0 || stats.InFlight != 0 {
		t.Fatalf("Close returned before pending body handler settled: %+v", stats)
	}
}

type blockedBodyReader struct {
	started chan struct{}
	release chan struct{}
	sent    bool
}

func (r *blockedBodyReader) Read(buffer []byte) (int, error) {
	if !r.sent {
		r.sent = true
		data := []byte(`{"input":`)
		n := copy(buffer, data)
		close(r.started)
		return n, nil
	}
	<-r.release
	return 0, io.EOF
}

func requestBrokerNoFatal(server *Server) (*http.Response, error) {
	request, err := http.NewRequest(http.MethodPost, server.Endpoint()+"/responses", strings.NewReader(`{"input":"hi"}`))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+server.Token())
	request.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(request)
}

func response(status int, body, contentType string) *http.Response {
	header := make(http.Header)
	if contentType != "" {
		header.Set("Content-Type", contentType)
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}
}
