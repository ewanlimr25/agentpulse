import type { Project, Run, Span, Topology, BudgetRule, BudgetAlert } from "./types";

const BASE_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  const body = await res.json();
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
  list: (projectId: string, limit = 50, offset = 0) =>
    apiFetch<{ runs: Run[]; limit: number; offset: number }>(
      `/api/v1/projects/${projectId}/runs?limit=${limit}&offset=${offset}`
    ),
  get: (runId: string) => apiFetch<Run>(`/api/v1/runs/${runId}`),
  spans: (runId: string) => apiFetch<Span[]>(`/api/v1/runs/${runId}/spans`),
  topology: (runId: string) =>
    apiFetch<Topology>(`/api/v1/runs/${runId}/topology`),
};

// ── Budget ────────────────────────────────────────────────────────────────────

export const budgetApi = {
  listRules: (projectId: string) =>
    apiFetch<BudgetRule[]>(`/api/v1/projects/${projectId}/budget/rules`),
  createRule: (projectId: string, rule: Omit<BudgetRule, "ID" | "ProjectID" | "CreatedAt" | "UpdatedAt">) =>
    apiFetch<BudgetRule>(`/api/v1/projects/${projectId}/budget/rules`, {
      method: "POST",
      body: JSON.stringify(rule),
    }),
  updateRule: (projectId: string, ruleId: string, data: { Enabled?: boolean; Name?: string; ThresholdUSD?: number; Action?: string }) =>
    apiFetch<BudgetRule>(`/api/v1/projects/${projectId}/budget/rules/${ruleId}`, {
      method: "PUT",
      body: JSON.stringify(data),
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
};
