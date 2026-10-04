package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: webhook-receiver PORT_FILE RECEIVED_BODY_FILE")
		os.Exit(2)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	if err := os.WriteFile(os.Args[1], []byte(fmt.Sprintf("%d\n", port)), 0o600); err != nil {
		fatal(err)
	}
	received := make(chan struct{}, 1)
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(writer, "POST required", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(request.Body, 4<<20))
		if err != nil {
			http.Error(writer, "read body", http.StatusBadRequest)
			return
		}
		file, err := os.OpenFile(os.Args[2], os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			http.Error(writer, "body already stored", http.StatusConflict)
			return
		}
		_, writeErr := file.Write(body)
		syncErr := file.Sync()
		closeErr := file.Close()
		if writeErr != nil || syncErr != nil || closeErr != nil {
			http.Error(writer, "store body", http.StatusInternalServerError)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
		_, _ = writer.Write([]byte("accepted"))
		select {
		case received <- struct{}{}:
		default:
		}
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 2 * time.Second}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	select {
	case <-received:
	case err := <-serveErr:
		if err != http.ErrServerClosed {
			fatal(err)
		}
	case <-time.After(30 * time.Second):
		fatal(fmt.Errorf("timed out waiting for webhook"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
