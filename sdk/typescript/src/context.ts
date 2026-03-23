/**
 * Run-ID and project-ID context propagation using OTel's native Context API.
 *
 * All state flows through the OTel context (not module-scoped variables).
 * This guarantees correct behaviour across:
 *   - Async tasks (AsyncLocalStorage via OTel context-async-hooks)
 *   - Dual ESM/CJS loading (single OTel API instance = single context)
 *
 * Edge runtime note: setRunId() requires AsyncLocalStorage. In environments
 * where it is unavailable (Cloudflare Workers without nodejs_compat),
 * setRunId() will throw. Pass runId explicitly to span functions instead.
 */

import { context, createContextKey } from '@opentelemetry/api'

const RUN_ID_KEY = createContextKey('agentpulse.run_id')
const PROJECT_ID_KEY = createContextKey('agentpulse.project_id')
const SESSION_ID_KEY = createContextKey('agentpulse.session_id')

function hasAsyncLocalStorage(): boolean {
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const { AsyncLocalStorage } = require('node:async_hooks')
    return typeof AsyncLocalStorage === 'function'
  } catch {
    return false
  }
}

// Module-level fallback storage for environments with AsyncLocalStorage
let _activeRunId: string | undefined
let _activeProjectId: string | undefined
let _activeSessionId: string | undefined

/**
 * Pin a specific runId for the current async context.
 * @throws Error if AsyncLocalStorage is unavailable (edge runtimes without nodejs_compat).
 */
export function setRunId(runId: string): void {
  if (!hasAsyncLocalStorage()) {
    throw new Error(
      'setRunId() requires AsyncLocalStorage, which is unavailable in this runtime. ' +
      'In Cloudflare Workers, enable the nodejs_compat flag. ' +
      'Alternatively, pass runId explicitly to each span function: llmCall({ model, runId }).'
    )
  }
  _activeRunId = runId
}

export function getRunId(explicitRunId?: string): string {
  if (explicitRunId) return explicitRunId
  // Try OTel context first
  const fromCtx = context.active().getValue(RUN_ID_KEY) as string | undefined
  if (fromCtx) return fromCtx
  // Fall back to module-level (only set when AsyncLocalStorage is available)
  if (_activeRunId) return _activeRunId
  // Auto-generate
  return generateRunId()
}

export function getProjectId(): string | undefined {
  const fromCtx = context.active().getValue(PROJECT_ID_KEY) as string | undefined
  return fromCtx ?? (_activeProjectId || undefined)
}

export function setProjectId(projectId: string): void {
  _activeProjectId = projectId
}

export function resetRun(): void {
  _activeRunId = undefined
}

/**
 * Generate a new random session ID (UUID v4).
 * Convenience helper — pass the result to setSessionId().
 */
export function generateSessionId(): string {
  if (typeof crypto !== 'undefined' && crypto.randomUUID) {
    return crypto.randomUUID()
  }
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, c => {
    const r = (Math.random() * 16) | 0
    const v = c === 'x' ? r : (r & 0x3) | 0x8
    return v.toString(16)
  })
}

/**
 * Pin a session ID for the current async context.
 * All spans created after this call will carry `agentpulse.session_id`,
 * grouping multiple runs into a single conversation/session in the UI.
 *
 * Sessions are opt-in — omitting this means runs are tracked individually
 * and won't appear in the Sessions tab.
 */
export function setSessionId(sessionId: string): void {
  _activeSessionId = sessionId
}

/**
 * Return the current session ID, or undefined if not set.
 * Unlike runId, session ID is never auto-generated.
 */
export function getSessionId(): string | undefined {
  const fromCtx = context.active().getValue(SESSION_ID_KEY) as string | undefined
  return fromCtx ?? (_activeSessionId || undefined)
}

/** Clear the current session ID. */
export function resetSession(): void {
  _activeSessionId = undefined
}

function generateRunId(): string {
  if (typeof crypto !== 'undefined' && crypto.randomUUID) {
    return crypto.randomUUID()
  }
  // Node.js 14 fallback
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, c => {
    const r = (Math.random() * 16) | 0
    const v = c === 'x' ? r : (r & 0x3) | 0x8
    return v.toString(16)
  })
}
