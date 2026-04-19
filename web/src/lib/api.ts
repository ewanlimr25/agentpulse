import type { Project, Run, RunLoop, RunsListResponse, Span, Topology, BudgetRule, BudgetAlert, RecentBudgetAlert, SpanEval, SpanEvalGroup, RunEvalSummary, EvalConfig, AlertRule, AlertEvent, RecentAlertEvent, ToolStats, AgentCostStats, AnalyticsWindow, Session, SessionsListResponse, UserStats, UsersListResponse, RunComparison, ReplayBundle, SearchResponse, ProjectPIIConfig, PIICustomRule, SpanFeedback, FeedbackRequest, ProjectHealth, PlaygroundSession, PlaygroundSessionsListResponse, PlaygroundVariant, PlaygroundExecution, PlaygroundMessage, ModelInfo, ModelStatsResponse } from "./types";
import { getApiKey } from "./api-keys";

const BASE_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

/** Thrown when the API returns 401 or 403. */
export class AuthError extends Error {
  status: number;
  projectId: string | null;
  constructor(status: number, message: string, projectId: string | null) {
    super(message);
    this.name = "AuthError";
    this.status = status;
    this.projectId = projectId;
  }
}

/** Extract the projectID from a project-scoped API path, or null. */
function extractProjectId(path: string): string | null {
  const m = path.match(/\/api\/v1\/projects\/([^/]+)/);
  return m ? m[1] : null;
}

interface ApiFetchOptions extends RequestInit {
  /** Explicit project ID to use for Bearer token lookup. Use when the path
   *  does not contain /api/v1/projects/{id}/ (e.g. run-scoped routes). */
  projectId?: string;
}

async function apiFetch<T>(path: string, init?: ApiFetchOptions): Promise<T> {
  const projectId = init?.projectId ?? extractProjectId(path);
  const apiKey = projectId ? getApiKey(projectId) : null;

  const { projectId: _, headers: extraHeaders, ...fetchInit } = init ?? {};
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(extraHeaders as Record<string, string> | undefined),
  };
  if (apiKey) {
    headers["Authorization"] = `Bearer ${apiKey}`;
  }

  const res = await fetch(`${BASE_URL}${path}`, {
    headers,
    ...fetchInit,
  });
  const body = await res.json();
  if (res.status === 401 || res.status === 403) {
    throw new AuthError(res.status, body.error ?? `HTTP ${res.status}`, projectId);
  }
  if (!res.ok) {
    throw new Error(body.error ?? `HTTP ${res.status}`);
  }
  return body.data as T;
}

// ── Projects ─────────────────────────────────────────────────────────────────

export const projectsApi = {
  list: () => apiFetch<Project[]>("/api/v1/projects"),
  get: (id: string) => apiFetch<Project>(`/api/v1/projects/${id}`),
  create: (name: string) =>
    apiFetch<{ project: Project; api_key: string; admin_key?: string }>("/api/v1/projects", {
      method: "POST",
      body: JSON.stringify({ name }),
    }),
};

// ── Runs ─────────────────────────────────────────────────────────────────────

export const runsApi = {
  list: (projectId: string, limit = 20, offset = 0, tags: string[] = []) =>
    apiFetch<RunsListResponse>(
      `/api/v1/projects/${projectId}/runs?limit=${limit}&offset=${offset}${tags.map((t) => `&tag=${encodeURIComponent(t)}`).join("")}`
    ),
  get: (runId: string, projectId: string) =>
    apiFetch<Run>(`/api/v1/runs/${runId}`, { projectId }),
  spans: (runId: string, projectId: string) =>
    apiFetch<Span[]>(`/api/v1/runs/${runId}/spans`, { projectId }),
  topology: (runId: string, projectId: string) =>
    apiFetch<Topology>(`/api/v1/runs/${runId}/topology`, { projectId }),
  compare: (projectId: string, a: string, b: string) =>
    apiFetch<RunComparison>(
      `/api/v1/projects/${projectId}/runs/compare?a=${encodeURIComponent(a)}&b=${encodeURIComponent(b)}`
    ),
  fetchSpan: (runId: string, spanId: string, projectId: string) =>
    apiFetch<Span>(`/api/v1/runs/${runId}/spans/${spanId}`, { projectId }),
  replayBundle: (runId: string, projectId: string) =>
    apiFetch<ReplayBundle>(`/api/v1/runs/${runId}/replay-bundle`, { projectId }),
};

// ── Run Tags & Annotations ────────────────────────────────────────────────────

export const runTagsApi = {
  listTags: (runId: string, projectId: string) =>
    apiFetch<{ tags: string[] }>(`/api/v1/runs/${runId}/tags`, { projectId })
      .then((r) => r.tags),
  addTag: (runId: string, tag: string, projectId: string) =>
    apiFetch<void>(`/api/v1/runs/${runId}/tags`, {
      method: "POST",
      projectId,
      body: JSON.stringify({ tag }),
    }),
  removeTag: (runId: string, tag: string, projectId: string) =>
    apiFetch<void>(`/api/v1/runs/${runId}/tags/${encodeURIComponent(tag)}`, {
      method: "DELETE",
      projectId,
    }),
  upsertAnnotation: (runId: string, note: string, projectId: string) =>
    apiFetch<void>(`/api/v1/runs/${runId}/annotation`, {
      method: "PUT",
      projectId,
      body: JSON.stringify({ note }),
    }),
  deleteAnnotation: (runId: string, projectId: string) =>
    apiFetch<void>(`/api/v1/runs/${runId}/annotation`, {
      method: "DELETE",
      projectId,
    }),
};

export const projectTagsApi = {
  listProjectTags: (projectId: string) =>
    apiFetch<{ tags: string[] }>(`/api/v1/projects/${projectId}/tags`)
      .then((r) => r.tags),
};

// ── Evals ─────────────────────────────────────────────────────────────────────

export const evalsApi = {
  listByRun: (runId: string, projectId: string) =>
    apiFetch<SpanEval[]>(`/api/v1/runs/${runId}/evals`, { projectId }),
  listByRunGrouped: (runId: string, projectId: string) =>
    apiFetch<SpanEvalGroup[]>(`/api/v1/runs/${runId}/evals/grouped`, { projectId }),
  summaryByProject: (projectId: string) =>
    apiFetch<RunEvalSummary[]>(`/api/v1/projects/${projectId}/evals/summary`),
  listConfigs: (projectId: string) =>
    apiFetch<EvalConfig[]>(`/api/v1/projects/${projectId}/evals/config`),
  upsertConfig: (projectId: string, cfg: { eval_name: string; enabled: boolean; span_kind: string; prompt_template?: string; scope_filter?: Record<string, string[]>; judge_models?: string[] }) =>
    apiFetch<EvalConfig>(`/api/v1/projects/${projectId}/evals/config`, {
      method: "POST",
      body: JSON.stringify(cfg),
    }),
  deleteConfig: (projectId: string, evalName: string) =>
    apiFetch<{ deleted: string }>(`/api/v1/projects/${projectId}/evals/config/${encodeURIComponent(evalName)}`, {
      method: "DELETE",
    }),
};

// ── Budget ────────────────────────────────────────────────────────────────────

export const budgetApi = {
  listRules: (projectId: string) =>
    apiFetch<BudgetRule[]>(`/api/v1/projects/${projectId}/budget/rules`),
  createRule: (projectId: string, rule: Omit<BudgetRule, "ID" | "ProjectID" | "CreatedAt" | "UpdatedAt">) =>
    apiFetch<BudgetRule>(`/api/v1/projects/${projectId}/budget/rules`, {
      method: "POST",
      body: JSON.stringify({
        name: rule.Name,
        threshold_usd: rule.ThresholdUSD,
        action: rule.Action,
        scope: rule.Scope,
        webhook_url: rule.WebhookURL ?? null,
        enabled: rule.Enabled,
      }),
    }),
  updateRule: (projectId: string, ruleId: string, data: { Enabled?: boolean; Name?: string; ThresholdUSD?: number; Action?: string; Scope?: string }) =>
    apiFetch<BudgetRule>(`/api/v1/projects/${projectId}/budget/rules/${ruleId}`, {
      method: "PUT",
      body: JSON.stringify({
        name: data.Name,
        threshold_usd: data.ThresholdUSD,
        action: data.Action,
        scope: data.Scope,
        enabled: data.Enabled,
      }),
    }),
  deleteRule: (projectId: string, ruleId: string) =>
    apiFetch<{ deleted: string }>(
      `/api/v1/projects/${projectId}/budget/rules/${ruleId}`,
      { method: "DELETE" }
    ),
  listAlerts: (projectId: string, limit = 100) =>
    apiFetch<BudgetAlert[]>(
      `/api/v1/projects/${projectId}/budget/alerts?limit=${limit}`
    ),
  listRecentAlerts: (projectId: string, limit = 20) =>
    apiFetch<RecentBudgetAlert[]>(`/api/v1/projects/${projectId}/budget/alerts/recent?limit=${limit}`),
};

// ── Signal Alerts ─────────────────────────────────────────────────────────────

export const alertsApi = {
  listRules: (projectId: string) =>
    apiFetch<AlertRule[]>(`/api/v1/projects/${projectId}/alerts/rules`),
  createRule: (projectId: string, body: {
    name: string; signal_type: string; threshold: number; compare_op: string;
    window_seconds: number; scope_filter?: string; webhook_url?: string; enabled: boolean;
  }) =>
    apiFetch<AlertRule>(`/api/v1/projects/${projectId}/alerts/rules`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  updateRule: (projectId: string, ruleId: string, body: {
    name: string; signal_type: string; threshold: number; compare_op: string;
    window_seconds: number; scope_filter?: string; webhook_url?: string; enabled: boolean;
  }) =>
    apiFetch<AlertRule>(`/api/v1/projects/${projectId}/alerts/rules/${ruleId}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  deleteRule: (projectId: string, ruleId: string) =>
    apiFetch<{ deleted: string }>(
      `/api/v1/projects/${projectId}/alerts/rules/${ruleId}`,
      { method: "DELETE" }
    ),
  listEvents: (projectId: string, limit = 100) =>
    apiFetch<AlertEvent[]>(`/api/v1/projects/${projectId}/alerts/events?limit=${limit}`),
  listRecentEvents: (projectId: string, limit = 20) =>
    apiFetch<RecentAlertEvent[]>(`/api/v1/projects/${projectId}/alerts/events/recent?limit=${limit}`),
};

// ── Loops ─────────────────────────────────────────────────────────────────────

export const loopsApi = {
  listByRun: (runId: string, projectId: string) =>
    apiFetch<RunLoop[]>(`/api/v1/runs/${runId}/loops`, { projectId }),
};

// ── Analytics ─────────────────────────────────────────────────────────────────

export const analyticsApi = {
  toolStats: (projectId: string, window: AnalyticsWindow = "24h") =>
    apiFetch<{ tools: ToolStats[]; window: string }>(
      `/api/v1/projects/${projectId}/analytics/tools?window=${window}`
    ),
  agentCostStats: (projectId: string, window: AnalyticsWindow = "24h") =>
    apiFetch<{ agents: AgentCostStats[]; window: string }>(
      `/api/v1/projects/${projectId}/analytics/agents?window=${window}`
    ),
  modelStats: (projectId: string, window: AnalyticsWindow = "24h") =>
    apiFetch<ModelStatsResponse>(
      `/api/v1/projects/${projectId}/analytics/models?window=${window}`
    ),
};

// ── Sessions ──────────────────────────────────────────────────────────────────

export const sessionsApi = {
  list: (projectId: string, limit = 50, offset = 0) =>
    apiFetch<SessionsListResponse>(
      `/api/v1/projects/${projectId}/sessions?limit=${limit}&offset=${offset}`
    ),
  get: (projectId: string, sessionId: string) =>
    apiFetch<Session>(`/api/v1/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}`),
  listRuns: (projectId: string, sessionId: string) =>
    apiFetch<Run[]>(`/api/v1/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}/runs`),
};

// ── Users ─────────────────────────────────────────────────────────────────────

export const usersApi = {
  list: (projectId: string, limit = 50, offset = 0) =>
    apiFetch<UsersListResponse>(
      `/api/v1/projects/${projectId}/users?limit=${limit}&offset=${offset}`
    ),
};

// ── Settings ──────────────────────────────────────────────────────────────────

export const settingsApi = {
  get: (projectId: string) =>
    apiFetch<ProjectPIIConfig>(`/api/v1/projects/${projectId}/settings`),

  update: (
    projectId: string,
    body: { pii_redaction_enabled: boolean; pii_custom_rules: PIICustomRule[] },
    adminKey: string
  ) =>
    apiFetch<ProjectPIIConfig>(`/api/v1/projects/${projectId}/settings`, {
      method: "PUT",
      headers: { "X-Admin-Key": adminKey },
      body: JSON.stringify(body),
    }),
};

// ── Span Feedback (Human-in-the-Loop Evals) ──────────────────────────────────

export const spanFeedbackApi = {
  upsert: (projectId: string, spanId: string, req: FeedbackRequest) =>
    apiFetch<SpanFeedback>(`/api/v1/projects/${projectId}/spans/${spanId}/feedback`, {
      method: "POST",
      body: JSON.stringify(req),
    }),
  get: (projectId: string, spanId: string) =>
    apiFetch<SpanFeedback>(`/api/v1/projects/${projectId}/spans/${spanId}/feedback`),
  delete: (projectId: string, spanId: string) =>
    apiFetch<void>(`/api/v1/projects/${projectId}/spans/${spanId}/feedback`, {
      method: "DELETE",
    }),
  listByRun: (projectId: string, runId: string) =>
    apiFetch<SpanFeedback[]>(`/api/v1/projects/${projectId}/runs/${runId}/feedback`),
};

// ── Health ───────────────────────────────────────────────────────────────────
export const healthApi = {
  status: (projectId: string) =>
    apiFetch<ProjectHealth>(`/api/v1/projects/${projectId}/health`),
};

// ── Search ─────────────────────────────────────────────────────────────────────

export const searchApi = {
  search: (projectId: string, params: {
    q: string;
    span_kind?: string;
    from?: string;
    to?: string;
    limit?: number;
    offset?: number;
  }) => {
    const qs = new URLSearchParams({ q: params.q });
    if (params.span_kind) qs.set('span_kind', params.span_kind);
    if (params.from) qs.set('from', params.from);
    if (params.to) qs.set('to', params.to);
    qs.set('limit', String(params.limit ?? 20));
    qs.set('offset', String(params.offset ?? 0));
    return apiFetch<SearchResponse>(`/api/v1/projects/${projectId}/search?${qs}`);
  },
};

// ── Prompt Playground ────────────────────────────────────────────────────────

export const playgroundApi = {
  listSessions: (projectId: string, limit = 20, offset = 0) =>
    apiFetch<PlaygroundSessionsListResponse>(
      `/api/v1/projects/${projectId}/playground/sessions?limit=${limit}&offset=${offset}`
    ),
  getSession: (projectId: string, sessionId: string) =>
    apiFetch<PlaygroundSession>(
      `/api/v1/projects/${projectId}/playground/sessions/${sessionId}`
    ),
  createSession: (projectId: string, body: {
    name: string;
    source_span_id?: string;
    source_run_id?: string;
    variants: Array<{
      label: string;
      model_id: string;
      system?: string;
      messages: PlaygroundMessage[];
      temperature?: number;
      max_tokens?: number;
    }>;
  }) =>
    apiFetch<PlaygroundSession>(`/api/v1/projects/${projectId}/playground/sessions`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  updateVariant: (projectId: string, sessionId: string, variantId: string, body: {
    label?: string;
    model_id?: string;
    system?: string;
    messages?: PlaygroundMessage[];
    temperature?: number | null;
    max_tokens?: number | null;
  }) =>
    apiFetch<PlaygroundVariant>(
      `/api/v1/projects/${projectId}/playground/sessions/${sessionId}/variants/${variantId}`,
      { method: "PUT", body: JSON.stringify(body) }
    ),
  runVariant: (projectId: string, sessionId: string, variantId: string) =>
    apiFetch<PlaygroundExecution>(
      `/api/v1/projects/${projectId}/playground/sessions/${sessionId}/variants/${variantId}/run`,
      { method: "POST" }
    ),
  deleteSession: (projectId: string, sessionId: string) =>
    apiFetch<{ deleted: string }>(
      `/api/v1/projects/${projectId}/playground/sessions/${sessionId}`,
      { method: "DELETE" }
    ),
};

// ── Models ──────────────────────────────────────────────────────────────────

export const modelsApi = {
  list: () => apiFetch<ModelInfo[]>("/api/v1/models"),
};
