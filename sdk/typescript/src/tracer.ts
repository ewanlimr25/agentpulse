/**
 * AgentPulse tracer initialization.
 * One-line setup: initTracer({ projectId: 'xxx' })
 */

import { trace, type Tracer } from '@opentelemetry/api'
import type { AgentPulseConfig } from './config.js'
import { loadConfig, resolvedEndpoint } from './config.js'
import { setProjectId } from './context.js'

// eslint-disable-next-line @typescript-eslint/no-explicit-any
let _provider: any

export function initTracer(configOrOverrides?: Partial<AgentPulseConfig>): Tracer {
  const config = loadConfig(configOrOverrides)

  if (_provider) {
    console.warn('[AgentPulse] initTracer() called more than once. Ignoring duplicate call.')
    return trace.getTracer('agentpulse')
  }

  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const { NodeTracerProvider } = require('@opentelemetry/sdk-trace-node')
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const { BatchSpanProcessor, SimpleSpanProcessor } = require('@opentelemetry/sdk-trace-base')
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const { Resource } = require('@opentelemetry/resources')
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const { SEMRESATTRS_SERVICE_NAME } = require('@opentelemetry/semantic-conventions')

  const resource = new Resource({
    [SEMRESATTRS_SERVICE_NAME]: config.serviceName,
    'agentpulse.project_id': config.projectId,
  })

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  let exporter: any
  const endpoint = resolvedEndpoint(config)

  if (config.protocol === 'grpc') {
    try {
      // eslint-disable-next-line @typescript-eslint/no-require-imports
      const { OTLPTraceExporter } = require('@opentelemetry/exporter-trace-otlp-grpc')
      // eslint-disable-next-line @typescript-eslint/no-require-imports
      const grpc = require('@grpc/grpc-js')
      exporter = new OTLPTraceExporter({
        url: endpoint,
        credentials: config.insecure ? grpc.credentials.createInsecure() : undefined,
      })
    } catch {
      throw new Error(
        '[AgentPulse] gRPC exporter not installed. ' +
        'Run: npm install @opentelemetry/exporter-trace-otlp-grpc @grpc/grpc-js\n' +
        'Or set protocol: "http" in your config.'
      )
    }
  } else {
    try {
      // eslint-disable-next-line @typescript-eslint/no-require-imports
      const { OTLPTraceExporter } = require('@opentelemetry/exporter-trace-otlp-http')
      exporter = new OTLPTraceExporter({ url: endpoint })
    } catch {
      throw new Error(
        '[AgentPulse] HTTP exporter not installed. ' +
        'Run: npm install @opentelemetry/exporter-trace-otlp-http'
      )
    }
  }

  const processor = config.batchExport
    ? new BatchSpanProcessor(exporter, { exportTimeoutMillis: config.exportTimeoutMs })
    : new SimpleSpanProcessor(exporter)

  _provider = new NodeTracerProvider({ resource })
  _provider.addSpanProcessor(processor)
  _provider.register()

  setProjectId(config.projectId)

  return trace.getTracer('agentpulse')
}

export async function shutdown(): Promise<void> {
  if (_provider) {
    await _provider.shutdown()
    _provider = undefined
  }
}
