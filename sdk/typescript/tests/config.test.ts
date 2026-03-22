import { describe, it, expect, afterEach } from 'vitest'
import { loadConfig, resolvedEndpoint } from '../src/config.js'

describe('loadConfig', () => {
  const originalEnv = { ...process.env }

  afterEach(() => {
    process.env = { ...originalEnv }
  })

  it('throws when projectId is missing', () => {
    delete process.env.AGENTPULSE_PROJECT_ID
    expect(() => loadConfig()).toThrow('projectId is required')
  })

  it('reads projectId from env', () => {
    process.env.AGENTPULSE_PROJECT_ID = 'env-project'
    const config = loadConfig()
    expect(config.projectId).toBe('env-project')
  })

  it('programmatic projectId overrides env', () => {
    process.env.AGENTPULSE_PROJECT_ID = 'env-project'
    const config = loadConfig({ projectId: 'code-project' })
    expect(config.projectId).toBe('code-project')
  })

  it('defaults protocol to grpc', () => {
    const config = loadConfig({ projectId: 'test' })
    expect(config.protocol).toBe('grpc')
  })

  it('reads protocol from env', () => {
    process.env.AGENTPULSE_PROTOCOL = 'http'
    const config = loadConfig({ projectId: 'test' })
    expect(config.protocol).toBe('http')
  })

  it('throws on invalid protocol', () => {
    expect(() => loadConfig({ projectId: 'test', protocol: 'websocket' as 'grpc' }))
      .toThrow("protocol must be 'grpc' or 'http'")
  })

  it('config is immutable', () => {
    const config = loadConfig({ projectId: 'test' })
    expect(() => { (config as unknown as Record<string, unknown>).projectId = 'changed' }).toThrow()
  })

  it('defaults batchExport to true', () => {
    const config = loadConfig({ projectId: 'test' })
    expect(config.batchExport).toBe(true)
  })
})

describe('resolvedEndpoint', () => {
  it('returns explicit endpoint when provided', () => {
    const config = loadConfig({ projectId: 'test', endpoint: 'custom:4317' })
    expect(resolvedEndpoint(config)).toBe('custom:4317')
  })

  it('returns grpc default when no endpoint', () => {
    const config = loadConfig({ projectId: 'test', protocol: 'grpc', endpoint: '' })
    expect(resolvedEndpoint(config)).toBe('localhost:4317')
  })

  it('returns http default when no endpoint', () => {
    const config = loadConfig({ projectId: 'test', protocol: 'http', endpoint: '' })
    expect(resolvedEndpoint(config)).toBe('http://localhost:4318')
  })
})
