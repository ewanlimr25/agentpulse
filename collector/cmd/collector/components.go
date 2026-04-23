package main

import (
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension"
	"go.opentelemetry.io/collector/exporter/debugexporter"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor/batchprocessor"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/collector/service/telemetry/otelconftelemetry"

	"github.com/agentpulse/agentpulse/collector/exporter/clickhouseexporter"
	"github.com/agentpulse/agentpulse/collector/exporter/topologyexporter"
	"github.com/agentpulse/agentpulse/collector/processor/agentsemanticproc"
	"github.com/agentpulse/agentpulse/collector/processor/authenforceproc"
	"github.com/agentpulse/agentpulse/collector/processor/budgetenforceproc"
	"github.com/agentpulse/agentpulse/collector/processor/piimaskerproc"
	"github.com/agentpulse/agentpulse/collector/processor/ratelimitproc"
)

// components returns the full set of factories for the AgentPulse collector.
func components() (otelcol.Factories, error) {
	var factories otelcol.Factories
	var err error

	// Extensions
	factories.Extensions, err = otelcol.MakeFactoryMap(
		healthcheckextension.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	// Receivers
	factories.Receivers, err = otelcol.MakeFactoryMap(
		otlpreceiver.NewFactory(), // OTLP gRPC + HTTP
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	// Processors
	factories.Processors, err = otelcol.MakeFactoryMap(
		authenforceproc.NewFactory(),    // ingest token authentication
		ratelimitproc.NewFactory(),      // per-project ingestion rate limiting
		agentsemanticproc.NewFactory(),  // agent span classification + cost
		piimaskerproc.NewFactory(),      // PII / secret redaction
		budgetenforceproc.NewFactory(),  // budget enforcement + alerting
		batchprocessor.NewFactory(),     // standard batching
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	// Exporters
	factories.Exporters, err = otelcol.MakeFactoryMap(
		clickhouseexporter.NewFactory(), // spans → ClickHouse
		topologyexporter.NewFactory(),   // spans → Postgres topology
		debugexporter.NewFactory(),      // stdout debug (dev only)
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	factories.Telemetry = otelconftelemetry.NewFactory()

	return factories, nil
}
