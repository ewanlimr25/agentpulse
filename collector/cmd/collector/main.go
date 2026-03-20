package main

import (
	"log"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/otelcol"
)

func main() {
	info := component.BuildInfo{
		Command:     "agentpulse-collector",
		Description: "AgentPulse OTel Collector with agent semantic extensions",
		Version:     "0.1.0",
	}

	cmd := otelcol.NewCommand(otelcol.CollectorSettings{BuildInfo: info})
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
