"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { ingestTokensApi } from "@/lib/api";
import { getApiKey } from "@/lib/api-keys";
import { useToast } from "@/components/toast/ToastContext";
import type { CreateIngestTokenResponse } from "@/lib/types";

interface Props {
  projectId: string;
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString();
}

interface NewTokenModalProps {
  response: CreateIngestTokenResponse;
  onDismiss: () => void;
}

function NewTokenModal({ response, onDismiss }: NewTokenModalProps) {
  const [copied, setCopied] = useState(false);

  function handleCopy() {
    navigator.clipboard.writeText(response.token).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60">
      <div className="w-full max-w-lg rounded-2xl border border-[var(--border)] bg-[var(--surface)] p-6 shadow-2xl flex flex-col gap-4">
        <div>
          <h2 className="text-base font-semibold text-[var(--text)] mb-1">Token Created</h2>
          <p className="text-sm font-semibold text-amber-400">
            This token will not be shown again.
          </p>
          <p className="text-xs text-[var(--text-muted)] mt-1">
            Copy and store it securely before dismissing this dialog.
          </p>
        </div>

        <div>
          <p className="text-xs text-[var(--text-muted)] mb-1">Label: <span className="text-[var(--text)]">{response.label}</span></p>
          <div className="flex items-center gap-2">
            <code className="flex-1 rounded-lg border border-[var(--border)] bg-[var(--surface-2)] px-3 py-2 text-xs font-mono text-[var(--text)] break-all">
              {response.token}
            </code>
            <button
              onClick={handleCopy}
              className="shrink-0 px-3 py-2 text-xs rounded-lg border border-[var(--border)] bg-[var(--surface-2)] hover:bg-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] transition-colors"
            >
              {copied ? "Copied!" : "Copy"}
            </button>
          </div>
        </div>

        <button
          onClick={onDismiss}
          className="mt-2 w-full py-2 rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white text-sm font-medium transition-colors"
        >
          I&apos;ve saved this token
        </button>
      </div>
    </div>
  );
}

export function IngestTokensSection({ projectId }: Props) {
  const queryClient = useQueryClient();
  const { addToast } = useToast();

  const apiKey = getApiKey(projectId) ?? "";
  const adminKey =
    typeof window !== "undefined"
      ? localStorage.getItem(`adminKey_${projectId}`)
      : null;

  const readOnly = !adminKey;

  const [showCreateForm, setShowCreateForm] = useState(false);
  const [labelInput, setLabelInput] = useState("");
  const [newTokenResponse, setNewTokenResponse] = useState<CreateIngestTokenResponse | null>(null);

  const { data: tokens, isLoading } = useQuery({
    queryKey: ["ingestTokens", projectId],
    queryFn: () => ingestTokensApi.list(projectId, apiKey, adminKey ?? ""),
    enabled: !readOnly,
  });

  const createMutation = useMutation({
    mutationFn: (label: string) =>
      ingestTokensApi.create(projectId, label, apiKey, adminKey ?? ""),
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ["ingestTokens", projectId] });
      setShowCreateForm(false);
      setLabelInput("");
      setNewTokenResponse(data);
    },
    onError: (err: Error) => {
      addToast({ title: "Failed to create token", message: err.message, variant: "halt" });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (tokenId: string) =>
      ingestTokensApi.delete(projectId, tokenId, apiKey, adminKey ?? ""),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["ingestTokens", projectId] });
      addToast({ title: "Token deleted", message: "The ingest token has been revoked.", variant: "notify" });
    },
    onError: (err: Error) => {
      addToast({ title: "Failed to delete token", message: err.message, variant: "halt" });
    },
  });

  function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    if (!labelInput.trim()) return;
    createMutation.mutate(labelInput.trim());
  }

  function handleDelete(tokenId: string, label: string) {
    if (!confirm(`Delete token "${label}"? This cannot be undone.`)) return;
    deleteMutation.mutate(tokenId);
  }

  return (
    <>
      {newTokenResponse && (
        <NewTokenModal
          response={newTokenResponse}
          onDismiss={() => setNewTokenResponse(null)}
        />
      )}

      <div className="flex flex-col gap-6">
        {/* No admin key notice */}
        {readOnly && (
          <div className="border border-amber-700/50 bg-amber-950/20 rounded-xl px-4 py-3 text-sm text-amber-300/90">
            Managing ingest tokens requires your Admin Key. Your Admin Key was shown once when this project was created. If you&apos;ve lost it, you&apos;ll need to recreate the project.
          </div>
        )}

        {/* Header row */}
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm font-semibold text-[var(--text)]">Ingest Tokens</p>
            <p className="text-xs text-[var(--text-muted)] mt-0.5">
              Bearer tokens used to authenticate data ingestion from your agents.
            </p>
          </div>
          {!readOnly && (
            <button
              onClick={() => setShowCreateForm(true)}
              disabled={showCreateForm || createMutation.isPending}
              className="text-xs px-3 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white font-medium transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            >
              + Generate Token
            </button>
          )}
        </div>

        {/* Inline create form */}
        {showCreateForm && (
          <form
            onSubmit={handleCreate}
            className="border border-[var(--border)] rounded-xl p-4 bg-[var(--surface-2)] flex flex-col gap-3"
          >
            <p className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wider">New Ingest Token</p>
            <div>
              <label className="block text-xs text-[var(--text-muted)] mb-1">Label</label>
              <input
                autoFocus
                type="text"
                required
                placeholder="e.g. production, staging"
                value={labelInput}
                onChange={(e) => setLabelInput(e.target.value)}
                className="w-full rounded-lg border border-[var(--border)] bg-[var(--surface)] text-[var(--text)] px-3 py-2 text-sm focus:outline-none focus:border-indigo-500"
              />
            </div>
            <div className="flex gap-2 justify-end">
              <button
                type="button"
                onClick={() => { setShowCreateForm(false); setLabelInput(""); }}
                className="px-3 py-1.5 text-xs rounded-lg border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] transition-colors"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={createMutation.isPending || !labelInput.trim()}
                className="px-3 py-1.5 text-xs rounded-lg bg-indigo-600 hover:bg-indigo-700 disabled:opacity-50 text-white font-medium transition-colors"
              >
                {createMutation.isPending ? "Creating..." : "Create"}
              </button>
            </div>
          </form>
        )}

        {/* Token table */}
        {readOnly ? null : isLoading ? (
          <p className="text-sm text-[var(--text-muted)] py-6">Loading tokens...</p>
        ) : !tokens || tokens.length === 0 ? (
          <p className="text-xs text-[var(--text-muted)] border border-[var(--border)] rounded-xl px-4 py-6 text-center">
            No ingest tokens yet. Generate a token to start sending data.
          </p>
        ) : (
          <div className="border border-[var(--border)] rounded-xl overflow-hidden">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-[var(--border)] bg-[var(--surface-2)]">
                  <th className="text-left px-4 py-2.5 text-xs font-medium text-[var(--text-muted)]">Label</th>
                  <th className="text-left px-4 py-2.5 text-xs font-medium text-[var(--text-muted)]">Created</th>
                  <th className="px-4 py-2.5" />
                </tr>
              </thead>
              <tbody>
                {tokens.map((token) => (
                  <tr key={token.id} className="border-t border-[var(--border)] first:border-0">
                    <td className="px-4 py-3 text-[var(--text)] font-medium text-sm">{token.label}</td>
                    <td className="px-4 py-3 text-xs text-[var(--text-muted)]">{formatDate(token.created_at)}</td>
                    <td className="px-4 py-3 text-right">
                      <button
                        onClick={() => handleDelete(token.id, token.label)}
                        disabled={deleteMutation.isPending}
                        className="text-xs px-2.5 py-1 rounded bg-red-950/30 hover:bg-red-950/50 text-red-400 transition-colors disabled:opacity-40"
                      >
                        Delete
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </>
  );
}
