import type { Project, Run, RunsListResponse, Span, Topology, BudgetRule, BudgetAlert, RecentBudgetAlert, SpanEval, RunEvalSummary, AlertRule, AlertEvent, RecentAlertEvent } from "./types";
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

async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const projectId = extractProjectId(path);
  const apiKey = projectId ? getApiKey(projectId) : null;

  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (apiKey) {
    headers["Authorization"] = `Bearer ${apiKey}`;
  }

  const res = await fetch(`${BASE_URL}${path}`, {
    headers,
    ...init,
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
    apiFetch<{ project: Project; api_key: string }>("/api/v1/projects", {
      method: "POST",
      body: JSON.stringify({ name }),
    }),
};

// ── Runs ─────────────────────────────────────────────────────────────────────

export const runsApi = {
  list: (projectId: string, limit = 20, offset = 0) =>
    apiFetch<RunsListResponse>(
      `/api/v1/projects/${projectId}/runs?limit=${limit}&offset=${offset}`
    ),
  get: (runId: string) => apiFetch<Run>(`/api/v1/runs/${runId}`),
  spans: (runId: string) => apiFetch<Span[]>(`/api/v1/runs/${runId}/spans`),
  topology: (runId: string) =>
    apiFetch<Topology>(`/api/v1/runs/${runId}/topology`),
};

// ── Evals ─────────────────────────────────────────────────────────────────────

export const evalsApi = {
  listByRun: (runId: string) =>
    apiFetch<SpanEval[]>(`/api/v1/runs/${runId}/evals`),
  summaryByProject: (projectId: string) =>
    apiFetch<RunEvalSummary[]>(`/api/v1/projects/${projectId}/evals/summary`),
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
  listRecentAlerts: (limit = 20) =>
    apiFetch<RecentBudgetAlert[]>(`/api/v1/budget/alerts/recent?limit=${limit}`),
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
  listRecentEvents: (limit = 20) =>
    apiFetch<RecentAlertEvent[]>(`/api/v1/alerts/events/recent?limit=${limit}`),
};
