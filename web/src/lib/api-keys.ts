/**
 * Per-project API key storage backed by localStorage.
 *
 * Keys are stored under "agentpulse:apiKeys" as a JSON object
 * mapping projectId → raw API key.
 *
 * Security note: localStorage is accessible to JavaScript running on the
 * same origin. This is acceptable for a single-tenant MVP. A future
 * production build should use HttpOnly cookies or a server-side session.
 */

const STORAGE_KEY = "agentpulse:apiKeys";

function loadAll(): Record<string, string> {
  if (typeof window === "undefined") return {};
  try {
    return JSON.parse(localStorage.getItem(STORAGE_KEY) ?? "{}");
  } catch {
    return {};
  }
}

function saveAll(keys: Record<string, string>): void {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(keys));
}

export function saveApiKey(projectId: string, key: string): void {
  const keys = loadAll();
  saveAll({ ...keys, [projectId]: key });
}

export function getApiKey(projectId: string): string | null {
  return loadAll()[projectId] ?? null;
}

export function removeApiKey(projectId: string): void {
  const rest = Object.fromEntries(
    Object.entries(loadAll()).filter(([k]) => k !== projectId)
  );
  saveAll(rest);
}

export function getAllApiKeys(): Record<string, string> {
  return loadAll();
}
