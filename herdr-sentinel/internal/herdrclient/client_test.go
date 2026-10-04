package herdrclient

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type testRequest struct {
	ID     string         `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

func fakeSocket(t *testing.T, handler func(net.Conn, testRequest)) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "herdr-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "h.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			request, err := readTestRequest(conn)
			if err != nil {
				t.Errorf("read request: %v", err)
				_ = conn.Close()
				continue
			}
			handler(conn, request)
			_ = conn.Close()
		}
	}()
	return path
}

func readTestRequest(conn net.Conn) (testRequest, error) {
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return testRequest{}, err
	}
	var request testRequest
	if err := json.Unmarshal(line, &request); err != nil {
		return testRequest{}, err
	}
	return request, nil
}

func reply(t *testing.T, conn net.Conn, request testRequest, result any) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"id": request.ID, "result": result})
	if err != nil {
		t.Errorf("marshal reply: %v", err)
		return
	}
	_, err = fmt.Fprintf(conn, "%s\n", data)
	if err != nil {
		t.Errorf("write reply: %v", err)
	}
}

func TestPingAndWorkspaceBindingUseHostIdentitiesWithoutFocus(t *testing.T) {
	path := fakeSocket(t, func(conn net.Conn, request testRequest) {
		switch request.Method {
		case "ping":
			reply(t, conn, request, map[string]any{"type": "pong", "version": "0.9.3", "protocol": 22})
		case "workspace.create":
			if request.Params["focus"] != false || request.Params["cwd"] != "/repo" || request.Params["label"] != "session-42" {
				t.Errorf("workspace.create params = %#v", request.Params)
			}
			reply(t, conn, request, map[string]any{
				"type":      "workspace_created",
				"workspace": map[string]any{"workspace_id": "w-host", "label": "session-42"},
				"tab":       map[string]any{"tab_id": "t-host"},
				"root_pane": map[string]any{
					"workspace_id": "w-host", "tab_id": "t-host", "pane_id": "p-host",
					"terminal_id": "term-host", "cwd": "/repo", "agent_status": "idle", "focused": false, "revision": 0,
				},
			})
		default:
			t.Errorf("unexpected method %q", request.Method)
		}
	})
	client := &Client{SocketPath: path, Timeout: time.Second}
	info, err := client.Ping(context.Background())
	if err != nil || info.Version != "0.9.3" || info.Protocol != 22 {
		t.Fatalf("Ping() = %+v, %v", info, err)
	}
	binding, err := client.CreateWorkspace(context.Background(), "/repo", "session-42")
	if err != nil {
		t.Fatal(err)
	}
	want := Binding{WorkspaceID: "w-host", TabID: "t-host", PaneID: "p-host", TerminalID: "term-host", Label: "session-42", CWD: "/repo"}
	if binding != want {
		t.Fatalf("CreateWorkspace() = %+v, want %+v", binding, want)
	}
}

func TestCreateWorkspaceRequiresExactReturnedLabelAndCWD(t *testing.T) {
	cases := []struct {
		name  string
		label string
		cwd   any
	}{
		{name: "label mismatch", label: "other", cwd: "/repo"},
		{name: "cwd mismatch", label: "session", cwd: "/other"},
		{name: "cwd missing", label: "session", cwd: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := fakeSocket(t, func(conn net.Conn, request testRequest) {
				reply(t, conn, request, map[string]any{
					"type":      "workspace_created",
					"workspace": map[string]any{"workspace_id": "w", "label": tc.label},
					"tab":       map[string]any{"tab_id": "t"},
					"root_pane": map[string]any{
						"workspace_id": "w", "tab_id": "t", "pane_id": "p", "terminal_id": "term", "cwd": tc.cwd,
					},
				})
			})
			_, err := (&Client{SocketPath: path}).CreateWorkspace(context.Background(), "/repo", "session")
			var delivery *DeliveryError
			if !errors.As(err, &delivery) || !delivery.MayHaveApplied {
				t.Fatalf("CreateWorkspace() error = %#v; want uncertain identity mismatch", err)
			}
		})
	}
}

func TestPaneValidatesPaneIdentityAndReadReturnsBoundObservation(t *testing.T) {
	path := fakeSocket(t, func(conn net.Conn, request testRequest) {
		if request.Method != "pane.get" {
			t.Errorf("pane.get request = %+v", request)
		}
		returnedPaneID := request.Params["pane_id"].(string)
		if returnedPaneID == "p-expected" {
			returnedPaneID = "p-other"
		}
		reply(t, conn, request, map[string]any{"type": "pane_info", "pane": map[string]any{
			"workspace_id": "w", "tab_id": "t", "pane_id": returnedPaneID, "terminal_id": "term",
			"cwd": "/repo", "foreground_cwd": "/repo/sub", "agent_status": "working", "focused": false, "revision": 8,
		}})
	})
	client := &Client{SocketPath: path}
	pane, err := client.Pane(context.Background(), "p-good")
	if err != nil || pane.PaneID != "p-good" || pane.TerminalID != "term" || pane.CWD != "/repo" || pane.ForegroundCWD != "/repo/sub" || pane.AgentStatus != "working" {
		t.Fatalf("Pane() = %+v, %v", pane, err)
	}
	_, err = client.Pane(context.Background(), "p-expected")
	var delivery *DeliveryError
	if !errors.As(err, &delivery) || delivery.MayHaveApplied {
		t.Fatalf("Pane() mismatch error = %#v; want definitive protocol error", err)
	}

	path = fakeSocket(t, func(conn net.Conn, request testRequest) {
		if request.Method != "pane.read" || request.Params["source"] != "recent" || request.Params["format"] != "text" {
			t.Errorf("pane.read request = %+v", request)
		}
		reply(t, conn, request, map[string]any{"type": "pane_read", "read": map[string]any{
			"workspace_id": "w", "tab_id": "t", "pane_id": "p-expected", "source": "recent", "format": "text",
			"text": "ready", "revision": 12, "truncated": false,
		}})
	})
	output, err := (&Client{SocketPath: path}).ReadPane(context.Background(), "p-expected")
	if err != nil || output.Text != "ready" || output.PaneID != "p-expected" || output.Revision != 12 || output.Truncated {
		t.Fatalf("ReadPane() = %+v, %v", output, err)
	}
}

func TestRunUsesOneAtomicInputAndCloseNeverClosesGroup(t *testing.T) {
	var calls atomic.Int32
	path := fakeSocket(t, func(conn net.Conn, request testRequest) {
		calls.Add(1)
		switch request.Method {
		case "pane.send_input":
			keys, ok := request.Params["keys"].([]any)
			if request.Params["pane_id"] != "p" || request.Params["text"] != "echo hello" || !ok || len(keys) != 1 || keys[0] != "enter" {
				t.Errorf("pane.send_input params = %#v", request.Params)
			}
		case "pane.send_keys":
			keys, ok := request.Params["keys"].([]any)
			if request.Params["pane_id"] != "p" || !ok || len(keys) != 1 || keys[0] != "ctrl+c" {
				t.Errorf("pane.send_keys params = %#v", request.Params)
			}
		case "workspace.close":
			if request.Params["workspace_id"] != "w" || request.Params["close_group"] != false {
				t.Errorf("workspace.close params = %#v", request.Params)
			}
		default:
			t.Errorf("unexpected method %q", request.Method)
		}
		reply(t, conn, request, map[string]any{"type": "ok"})
	})
	client := &Client{SocketPath: path}
	if err := client.Run(context.Background(), "p", "echo hello"); err != nil {
		t.Fatal(err)
	}
	if err := client.Interrupt(context.Background(), "p"); err != nil {
		t.Fatal(err)
	}
	if err := client.CloseWorkspace(context.Background(), "w"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatalf("RPC calls = %d, want 3", calls.Load())
	}
}

func TestHostErrorIsNotUncertainTransportFailure(t *testing.T) {
	path := fakeSocket(t, func(conn net.Conn, request testRequest) {
		_, _ = fmt.Fprintf(conn, `{"id":%q,"error":{"code":"pane_not_found","message":"gone"}}`+"\n", request.ID)
	})
	err := (&Client{SocketPath: path}).Run(context.Background(), "p", "echo hi")
	var hostError *HostError
	if !errors.As(err, &hostError) || hostError.Code != "pane_not_found" {
		t.Fatalf("Run() error = %#v; want host error", err)
	}
	var delivery *DeliveryError
	if errors.As(err, &delivery) {
		t.Fatalf("host rejection wrapped as uncertain delivery: %#v", err)
	}
}

func TestAmbiguousAndMalformedHostErrorResponsesAreUncertain(t *testing.T) {
	cases := []struct {
		name string
		body func(string) string
	}{
		{"result and error", func(id string) string {
			return fmt.Sprintf(`{"id":%q,"result":{"type":"ok"},"error":{"code":"conflict","message":"ambiguous"}}`, id)
		}},
		{"blank host error", func(id string) string {
			return fmt.Sprintf(`{"id":%q,"error":{"code":"","message":""}}`, id)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := fakeSocket(t, func(conn net.Conn, request testRequest) {
				_, _ = fmt.Fprintln(conn, tc.body(request.ID))
			})
			err := (&Client{SocketPath: path}).Run(context.Background(), "p", "do it")
			var delivery *DeliveryError
			if !errors.As(err, &delivery) || !delivery.MayHaveApplied {
				t.Fatalf("Run() error = %#v; want uncertain protocol error", err)
			}
		})
	}
}

func TestWrongIDPartialDisconnectAndOversizedResponseAreUncertain(t *testing.T) {
	cases := []struct {
		name    string
		handler func(net.Conn, testRequest)
		max     int64
	}{
		{"wrong id", func(conn net.Conn, request testRequest) {
			_, _ = fmt.Fprintln(conn, `{"id":"stale","result":{"type":"ok"}}`)
		}, 0},
		{"partial disconnect", func(conn net.Conn, _ testRequest) {
			_, _ = ioWriteString(conn, `{"id":`)
		}, 0},
		{"oversized response", func(conn net.Conn, request testRequest) {
			_, _ = fmt.Fprintf(conn, `{"id":%q,"result":{"type":"ok","padding":"%s"}}`+"\n", request.ID, strings.Repeat("x", 200))
		}, 64},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := fakeSocket(t, tc.handler)
			err := (&Client{SocketPath: path, Timeout: time.Second, MaxResponseBytes: tc.max}).Run(context.Background(), "p", "do it")
			var delivery *DeliveryError
			if !errors.As(err, &delivery) || !delivery.MayHaveApplied {
				t.Fatalf("Run() error = %#v; want uncertain delivery", err)
			}
		})
	}
}

func ioWriteString(writer net.Conn, value string) (int, error) {
	n, err := writer.Write([]byte(value))
	return n, err
}

func TestMutationIsNotRetriedAfterDisconnect(t *testing.T) {
	var calls atomic.Int32
	path := fakeSocket(t, func(_ net.Conn, _ testRequest) { calls.Add(1) })
	err := (&Client{SocketPath: path, Timeout: 300 * time.Millisecond}).Run(context.Background(), "p", "do it")
	var delivery *DeliveryError
	if !errors.As(err, &delivery) || !delivery.MayHaveApplied {
		t.Fatalf("Run() error = %#v; want uncertain delivery", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("request count = %d; mutations must not be retried", calls.Load())
	}
}

func TestContextCancellationClosesInFlightRequest(t *testing.T) {
	started := make(chan struct{})
	path := fakeSocket(t, func(conn net.Conn, _ testRequest) {
		close(started)
		buffer := make([]byte, 1)
		_, _ = conn.Read(buffer)
	})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- (&Client{SocketPath: path, Timeout: 5 * time.Second}).Run(ctx, "p", "do it") }()
	<-started
	cancel()
	select {
	case err := <-result:
		var delivery *DeliveryError
		if !errors.As(err, &delivery) || !delivery.MayHaveApplied || !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled Run() error = %#v; want uncertain context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("request did not stop after context cancellation")
	}
}

func TestListWorkspacesMatchesExactLabelAndCWD(t *testing.T) {
	path := fakeSocket(t, func(conn net.Conn, request testRequest) {
		if request.Method != "session.snapshot" {
			t.Errorf("method = %q", request.Method)
		}
		reply(t, conn, request, map[string]any{"type": "session_snapshot", "snapshot": map[string]any{
			"version": "0.9.3", "protocol": 22, "workspaces": []any{
				map[string]any{"workspace_id": "w1", "label": "wanted"},
				map[string]any{"workspace_id": "w2", "label": "wanted"},
				map[string]any{"workspace_id": "w3", "label": "other"},
			}, "panes": []any{
				map[string]any{"workspace_id": "w1", "tab_id": "t1", "pane_id": "p1", "terminal_id": "term1", "cwd": "/repo"},
				map[string]any{"workspace_id": "w2", "tab_id": "t2", "pane_id": "p2", "terminal_id": "term2", "cwd": "/repo/child"},
				map[string]any{"workspace_id": "w3", "tab_id": "t3", "pane_id": "p3", "terminal_id": "term3", "cwd": "/repo"},
			}, "tabs": []any{}, "layouts": []any{}, "agents": []any{},
		}})
	})
	matches, err := (&Client{SocketPath: path}).ListWorkspaces(context.Background(), "/repo", "wanted")
	if err != nil || len(matches) != 1 || matches[0].WorkspaceID != "w1" || matches[0].PaneID != "p1" {
		t.Fatalf("ListWorkspaces() = %+v, %v", matches, err)
	}
}

func TestTimeoutAndSocketPathAreExplicitAndBounded(t *testing.T) {
	if _, err := (&Client{}).Ping(context.Background()); err == nil || !strings.Contains(err.Error(), "explicit") {
		t.Fatalf("Ping with no socket path error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	path := fakeSocket(t, func(conn net.Conn, request testRequest) {
		<-ctx.Done()
	})
	_, err := (&Client{SocketPath: path, Timeout: time.Second}).Ping(ctx)
	var delivery *DeliveryError
	if !errors.As(err, &delivery) || delivery.MayHaveApplied {
		t.Fatalf("Ping deadline error = %#v; want bounded transport failure", err)
	}
}
