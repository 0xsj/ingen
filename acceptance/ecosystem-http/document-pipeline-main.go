package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	documentpipeline "ingen/examples/document-pipeline-lab/subject"
)

// The acceptance project uses graceful listener shutdown so Sorna can record
// an observed process exit code instead of a signal termination.
func main() {
	address := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	server := &http.Server{
		Addr:    *address,
		Handler: documentpipeline.NewHandler(documentpipeline.NewStore()),
	}
	shutdown, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-shutdown.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("graceful HTTP shutdown: %v", err)
		}
	}()

	log.Printf("document-pipeline subject listening on %s", *address)
	err := server.ListenAndServe()
	stop()
	<-shutdownDone
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
