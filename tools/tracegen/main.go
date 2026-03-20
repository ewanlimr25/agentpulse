// Command tracegen sends synthetic multi-agent OTLP trace data to a running
// AgentPulse collector. Useful for local development and smoke-testing.
//
// Usage:
//
//	go run ./tools/tracegen/... --runs 5 --agents 3
//	go run ./tools/tracegen/... --scenario simple-llm --runs 1
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	endpoint := flag.String("endpoint", "localhost:4317", "OTLP gRPC endpoint")
	runs := flag.Int("runs", 3, "Number of trace runs to emit")
	scenarioName := flag.String("scenario", "multi-agent-research", "Scenario to run: multi-agent-research | simple-llm | parallel-tools | all")
	projectID := flag.String("project-id", "demo-project", "Project ID embedded in spans")
	delay := flag.Duration("delay", 500*time.Millisecond, "Delay between runs")
	flag.Parse()

	scenariosToRun, err := resolveScenarios(*scenarioName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	ctx := context.Background()

	tp, err := newTracerProvider(ctx, *endpoint)
	if err != nil {
		log.Fatalf("init tracer provider: %v", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := tp.Shutdown(shutdownCtx); err != nil {
			log.Printf("tracer provider shutdown: %v", err)
		}
	}()

	tracer := otel.Tracer("tracegen")

	total := 0
	for i := 0; i < *runs; i++ {
		for name, fn := range scenariosToRun {
			log.Printf("run %d/%d  scenario=%s  project=%s", i+1, *runs, name, *projectID)
			if err := fn(ctx, tracer, *projectID); err != nil {
				log.Printf("scenario %s error: %v", name, err)
			}
			total++
		}
		if i < *runs-1 {
			time.Sleep(*delay)
		}
	}

	log.Printf("done — emitted %d trace(s)", total)
}

func resolveScenarios(name string) (map[string]scenario, error) {
	if name == "all" {
		return scenarios, nil
	}
	fn, ok := scenarios[name]
	if !ok {
		return nil, fmt.Errorf("unknown scenario %q — valid: multi-agent-research, simple-llm, parallel-tools, all", name)
	}
	return map[string]scenario{name: fn}, nil
}

func newTracerProvider(ctx context.Context, endpoint string) (*sdktrace.TracerProvider, error) {
	conn, err := grpc.NewClient(endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("grpc dial %s: %w", endpoint, err)
	}

	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("otlp exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("tracegen"),
			semconv.ServiceVersion("dev"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	return tp, nil
}
