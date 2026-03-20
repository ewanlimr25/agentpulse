package main

import (
	"go.opentelemetry.io/collector/exporter/debugexporter"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor/batchprocessor"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"

	"github.com/agentpulse/agentpulse/collector/exporter/clickhouseexporter"
	"github.com/agentpulse/agentpulse/collector/exporter/topologyexporter"
	"github.com/agentpulse/agentpulse/collector/processor/agentsemanticproc"
)

// components returns the full set of factories for the AgentPulse collector.
func components() (otelcol.Factories, error) {
	var factories otelcol.Factories
	var err error

	// Receivers
	factories.Receivers, err = otelcol.MakeFactoryMap(
		otlpreceiver.NewFactory(), // OTLP gRPC + HTTP
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	// Processors
	factories.Processors, err = otelcol.MakeFactoryMap(
		agentsemanticproc.NewFactory(), // agent span classification + cost
		batchprocessor.NewFactory(),    // standard batching
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

	return factories, nil
}
