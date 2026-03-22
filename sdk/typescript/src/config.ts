/**
 * AgentPulse SDK configuration.
 * All settings can be supplied programmatically or via environment variables.
 * Environment variables take precedence over parameter defaults.
 */

type Protocol = 'grpc' | 'http'

export interface AgentPulseConfig {
  readonly projectId: string
  readonly endpoint: string
  readonly serviceName: string
  readonly protocol: Protocol
  readonly insecure: boolean
  readonly batchExport: boolean
  readonly exportTimeoutMs: number
}

function getEnv(key: string, fallback = ''): string {
  if (typeof process !== 'undefined' && process.env) {
    return process.env[key] ?? fallback
  }
  return fallback
}

export function loadConfig(overrides: Partial<AgentPulseConfig> = {}): AgentPulseConfig {
  const projectId = overrides.projectId ?? getEnv('AGENTPULSE_PROJECT_ID')
  if (!projectId) {
    throw new Error(
      'AgentPulse projectId is required. ' +
      'Pass it to loadConfig({ projectId: "..." }) or set AGENTPULSE_PROJECT_ID.'
    )
  }

  const protocol = (overrides.protocol ?? getEnv('AGENTPULSE_PROTOCOL', 'grpc')) as Protocol
  if (protocol !== 'grpc' && protocol !== 'http') {
    throw new Error(`protocol must be 'grpc' or 'http', got ${JSON.stringify(protocol)}`)
  }

  const config: AgentPulseConfig = Object.freeze({
    projectId,
    endpoint: overrides.endpoint ?? getEnv('AGENTPULSE_ENDPOINT', ''),
    serviceName: overrides.serviceName ?? getEnv('AGENTPULSE_SERVICE', 'agentpulse-agent'),
    protocol,
    insecure: overrides.insecure ?? true,
    batchExport: overrides.batchExport ?? true,
    exportTimeoutMs: overrides.exportTimeoutMs ?? 30_000,
  })

  return config
}

export function resolvedEndpoint(config: AgentPulseConfig): string {
  if (config.endpoint) return config.endpoint
  return config.protocol === 'grpc' ? 'localhost:4317' : 'http://localhost:4318'
}
