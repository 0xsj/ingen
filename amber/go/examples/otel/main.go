package main

import (
	"context"
	"fmt"

	amber "github.com/0xsj/ingen/amber"
	amberotel "github.com/0xsj/ingen/amber/adapters/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(recorder),
	)
	defer func() {
		_ = provider.Shutdown(context.Background())
	}()

	ctx, span := provider.Tracer("amber/otel-example").Start(context.Background(), "process-order")
	provenance, err := amber.Start()
	if err != nil {
		return err
	}
	if err := amberotel.SetCurrentSpanAttributes(ctx, provenance); err != nil {
		return err
	}
	span.End()

	ended := recorder.Ended()
	if len(ended) != 1 {
		return fmt.Errorf("recorded %d ended spans, want 1", len(ended))
	}
	for _, attribute := range ended[0].Attributes() {
		if attribute.Key == "amber.execution_id" {
			fmt.Println("finished spans: 1")
			fmt.Println("Amber execution attribute present: true")
			return nil
		}
	}
	return fmt.Errorf("Amber execution attribute was not recorded")
}
