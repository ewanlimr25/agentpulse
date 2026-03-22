"use client";

import { useState } from "react";

interface Props {
  projectId: string;
  /** External error string set by the parent after key validation fails. */
  keyError?: string;
  /** Called with the raw key; resolves when done, never throws (errors come via keyError). */
  onKeySubmit: (key: string) => Promise<void>;
}

export function ApiKeyPrompt({ projectId: _projectId, keyError, onKeySubmit }: Props) {
  const [key, setKey] = useState("");
  const [localError, setLocalError] = useState("");
  const [pending, setPending] = useState(false);

  const displayError = keyError || localError;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const trimmed = key.trim();
    if (!trimmed) {
      setLocalError("API key cannot be empty.");
      return;
    }
    setLocalError("");
    setPending(true);
    await onKeySubmit(trimmed).finally(() => setPending(false));
  }

  return (
    <div className="flex items-center justify-center min-h-[40vh]">
      <div className="w-full max-w-md border border-[var(--border)] bg-[var(--surface)] rounded-xl px-8 py-8">
        <h2 className="text-lg font-semibold text-[var(--text)] mb-2">API Key Required</h2>
        <p className="text-sm text-[var(--text-muted)] mb-6">
          Enter the API key for this project. You received it when the project was created.
          If you lost it, regenerate it via the API.
        </p>

        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div>
            <label className="block text-xs text-[var(--text-muted)] mb-1">API Key</label>
            <input
              type="password"
              autoComplete="off"
              placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
              value={key}
              onChange={(e) => { setKey(e.target.value); setLocalError(""); }}
              className={`w-full bg-[var(--surface-2)] border rounded-lg px-3 py-2 text-sm text-[var(--text)] font-mono focus:outline-none focus:border-indigo-500 ${
                displayError ? "border-red-500" : "border-[var(--border)]"
              }`}
            />
            {displayError && <p className="text-xs text-red-400 mt-1">{displayError}</p>}
          </div>

          <button
            type="submit"
            disabled={pending}
            className="w-full bg-indigo-600 hover:bg-indigo-500 disabled:opacity-60 text-white text-sm font-medium py-2 rounded-lg transition-colors"
          >
            {pending ? "Verifying…" : "Unlock Project"}
          </button>
        </form>

        <p className="text-xs text-[var(--text-muted)] mt-6 font-mono bg-[var(--surface-2)] rounded-lg p-3">
          <span className="text-[var(--text-muted)]"># Retrieve your API key from project creation or generate via:</span><br />
          <span className="text-green-400">POST /api/v1/projects</span>
        </p>
      </div>
    </div>
  );
}
