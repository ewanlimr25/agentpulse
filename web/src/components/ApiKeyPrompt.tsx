"use client";

import { useState } from "react";
import { saveApiKey } from "@/lib/api-keys";

interface Props {
  projectId: string;
  onKeySubmit: () => void;
}

/**
 * Shown when a project page receives a 401. Lets the user paste their API key.
 * The key is saved to localStorage and queries are invalidated on submit.
 */
export function ApiKeyPrompt({ projectId, onKeySubmit }: Props) {
  const [key, setKey] = useState("");
  const [error, setError] = useState("");

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const trimmed = key.trim();
    if (!trimmed) {
      setError("API key cannot be empty.");
      return;
    }
    saveApiKey(projectId, trimmed);
    onKeySubmit();
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
            <label className="block text-xs text-[var(--text-muted)] mb-1">
              API Key
            </label>
            <input
              type="password"
              autoComplete="off"
              placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
              value={key}
              onChange={(e) => { setKey(e.target.value); setError(""); }}
              className="w-full bg-[var(--surface-2)] border border-[var(--border)] rounded-lg px-3 py-2 text-sm text-[var(--text)] font-mono focus:outline-none focus:border-indigo-500"
            />
            {error && <p className="text-xs text-red-400 mt-1">{error}</p>}
          </div>

          <button
            type="submit"
            className="w-full bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium py-2 rounded-lg transition-colors"
          >
            Unlock Project
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
