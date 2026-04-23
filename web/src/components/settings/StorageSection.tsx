"use client";

import { useEffect, useRef, useState } from "react";
import { useQuery, useMutation } from "@tanstack/react-query";
import { storageApi } from "@/lib/api";
import { useToast } from "@/components/toast/ToastContext";
import type { StorageStats, RetentionConfig, PurgeJob, DryRunResult } from "@/lib/types";

// ── Helpers ───────────────────────────────────────────────────────────────────

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString();
}

const RETENTION_OPTIONS = [7, 14, 30, 60, 90, 180, 365] as const;

const inputClass =
  "rounded-lg border border-[var(--border)] bg-[var(--surface)] text-[var(--text)] px-3 py-2 text-sm focus:outline-none focus:border-indigo-500";

const cardClass =
  "border border-[var(--border)] rounded-xl p-5 bg-[var(--surface)] flex flex-col gap-4";

// ── Status badge ──────────────────────────────────────────────────────────────

function StatusBadge({ status }: { status: PurgeJob["status"] }) {
  const map: Record<PurgeJob["status"], string> = {
    pending: "bg-yellow-900/40 text-yellow-300",
    running: "bg-blue-900/40 text-blue-300",
    completed: "bg-green-900/40 text-green-300",
    failed: "bg-red-900/40 text-red-300",
  };
  return (
    <span className={`text-xs font-medium px-2 py-0.5 rounded-full ${map[status]}`}>
      {status}
    </span>
  );
}

// ── Confirm modal ─────────────────────────────────────────────────────────────

interface ConfirmModalProps {
  isOpen: boolean;
  title: string;
  message: string;
  confirmLabel?: string;
  onConfirm: () => void;
  onCancel: () => void;
}

function ConfirmModal({
  isOpen,
  title,
  message,
  confirmLabel = "Delete",
  onConfirm,
  onCancel,
}: ConfirmModalProps) {
  useEffect(() => {
    if (!isOpen) return;
    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape") onCancel();
    }
    window.addEventListener("keydown", handleKey);
    return () => window.removeEventListener("keydown", handleKey);
  }, [isOpen, onCancel]);

  if (!isOpen) return null;

  return (
    <>
      <div
        className="fixed inset-0 bg-black/60 z-40"
        onClick={onCancel}
        aria-hidden="true"
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="confirm-modal-title"
        className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-md bg-[var(--surface)] border border-[var(--border)] rounded-xl p-6 flex flex-col gap-4"
      >
        <h2 id="confirm-modal-title" className="text-base font-semibold text-[var(--text)]">
          {title}
        </h2>
        <p className="text-sm text-[var(--text-muted)]">{message}</p>
        <div className="flex justify-end gap-2">
          <button
            onClick={onCancel}
            className="px-4 py-2 text-sm rounded-lg border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] transition-colors"
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            className="px-4 py-2 text-sm rounded-lg bg-red-600 hover:bg-red-700 text-white font-medium transition-colors"
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </>
  );
}

// ── Job Status display ────────────────────────────────────────────────────────

interface JobStatusCardProps {
  projectId: string;
  apiKey: string;
  jobId: string;
}

function JobStatusCard({ projectId, apiKey, jobId }: JobStatusCardProps) {
  const [job, setJob] = useState<PurgeJob | null>(null);
  const [error, setError] = useState<string | null>(null);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  function clearPolling() {
    if (intervalRef.current !== null) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
  }

  async function fetchJob() {
    try {
      const result = await storageApi.getPurgeJob(projectId, jobId, apiKey);
      setJob(result);
      if (result.status === "completed" || result.status === "failed") {
        clearPolling();
      }
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to fetch job status");
      clearPolling();
    }
  }

  useEffect(() => {
    void fetchJob();
    intervalRef.current = setInterval(() => {
      void fetchJob();
    }, 10000);
    return () => clearPolling();
    // fetchJob is stable across renders since it only uses the outer scope props/state
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [jobId]);

  const isTerminal = job?.status === "completed" || job?.status === "failed";

  return (
    <div className="border border-[var(--border)] rounded-xl p-4 mt-3 bg-[var(--surface-2)] flex flex-col gap-2">
      <div className="flex items-center justify-between">
        <p className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wider">
          Purge Job
        </p>
        {!isTerminal && (
          <button
            onClick={() => void fetchJob()}
            className="text-xs px-2.5 py-1 rounded bg-[var(--border)] hover:bg-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] transition-colors"
          >
            Check Status
          </button>
        )}
      </div>

      <p className="text-xs text-[var(--text-muted)] font-mono break-all">Job: {jobId}</p>

      {error && <p className="text-xs text-red-400">{error}</p>}

      {job && (
        <div className="flex flex-col gap-1.5">
          <div className="flex items-center gap-2">
            <span className="text-xs text-[var(--text-muted)]">Status:</span>
            <StatusBadge status={job.status} />
            {!isTerminal && (
              <span className="text-xs text-[var(--text-muted)] animate-pulse">polling every 10s…</span>
            )}
          </div>

          {isTerminal && (
            <div className="grid grid-cols-3 gap-2 mt-1">
              <div className="text-center">
                <p className="text-sm font-semibold text-[var(--text)]">
                  {job.spans_deleted.toLocaleString()}
                </p>
                <p className="text-xs text-[var(--text-muted)]">spans deleted</p>
              </div>
              <div className="text-center">
                <p className="text-sm font-semibold text-[var(--text)]">
                  {job.s3_keys_deleted.toLocaleString()}
                </p>
                <p className="text-xs text-[var(--text-muted)]">S3 keys</p>
              </div>
              <div className="text-center">
                <p className="text-sm font-semibold text-[var(--text)]">
                  {job.pg_rows_deleted.toLocaleString()}
                </p>
                <p className="text-xs text-[var(--text-muted)]">PG rows</p>
              </div>
            </div>
          )}

          {job.partial_failure && (
            <div className="border border-yellow-700/50 bg-yellow-950/20 rounded-lg px-3 py-2 text-xs text-yellow-300/90 mt-1">
              Purge completed with partial failure: some S3 objects or Postgres records may not have been cleaned up.
            </div>
          )}

          {job.error_msg && (
            <p className="text-xs text-red-400 mt-1">{job.error_msg}</p>
          )}
        </div>
      )}

      {!job && !error && (
        <p className="text-xs text-[var(--text-muted)]">Loading job status…</p>
      )}
    </div>
  );
}

// ── Storage Usage Card ────────────────────────────────────────────────────────

interface StorageUsageCardProps {
  projectId: string;
  apiKey: string;
}

function StorageUsageCard({ projectId, apiKey }: StorageUsageCardProps) {
  const [stats, setStats] = useState<StorageStats | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleRefresh() {
    setLoading(true);
    setError(null);
    try {
      const result = await storageApi.getStats(projectId, apiKey);
      setStats(result);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to load stats");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className={cardClass}>
      <div className="flex items-center justify-between">
        <p className="text-sm font-semibold text-[var(--text)]">Storage Usage</p>
        <button
          onClick={() => void handleRefresh()}
          disabled={loading}
          className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded-lg border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] hover:border-indigo-500 transition-colors disabled:opacity-50"
        >
          {loading ? (
            <svg className="animate-spin h-3.5 w-3.5" viewBox="0 0 24 24" fill="none">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
            </svg>
          ) : (
            <svg className="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path strokeLinecap="round" strokeLinejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
          )}
          Refresh
        </button>
      </div>

      {error && (
        <p className="text-sm text-red-400">{error}</p>
      )}

      {!stats && !loading && !error && (
        <p className="text-sm text-[var(--text-muted)]">Click Refresh to load stats.</p>
      )}

      {loading && !stats && (
        <div className="flex items-center gap-2 text-sm text-[var(--text-muted)]">
          <svg className="animate-spin h-4 w-4" viewBox="0 0 24 24" fill="none">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z" />
          </svg>
          Loading…
        </div>
      )}

      {stats && (
        <div className="flex flex-col gap-3">
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
            <div className="border border-[var(--border)] rounded-lg px-3 py-2.5">
              <p className="text-xs text-[var(--text-muted)] mb-1">Spans</p>
              <p className="text-sm font-medium text-[var(--text)]">
                {stats.span_row_count.toLocaleString()} rows
              </p>
              <p className="text-xs text-[var(--text-muted)]">~{formatBytes(stats.span_bytes_est)}</p>
            </div>
            <div className="border border-[var(--border)] rounded-lg px-3 py-2.5">
              <p className="text-xs text-[var(--text-muted)] mb-1">Topology</p>
              <p className="text-sm font-medium text-[var(--text)]">
                {stats.topology_rows.toLocaleString()} rows
              </p>
            </div>
            <div className="border border-[var(--border)] rounded-lg px-3 py-2.5">
              <p className="text-xs text-[var(--text-muted)] mb-1">S3 Payloads</p>
              <p className="text-sm font-medium text-[var(--text)]">
                {stats.s3_object_count.toLocaleString()} objects
              </p>
              <p className="text-xs text-[var(--text-muted)]">{formatBytes(stats.s3_bytes)}</p>
            </div>
            <div className="border border-[var(--border)] rounded-lg px-3 py-2.5">
              <p className="text-xs text-[var(--text-muted)] mb-1">Oldest data</p>
              <p className="text-sm font-medium text-[var(--text)]">
                {stats.oldest_span_at ? formatDate(stats.oldest_span_at) : "N/A"}
              </p>
            </div>
            <div className="border border-[var(--border)] rounded-lg px-3 py-2.5">
              <p className="text-xs text-[var(--text-muted)] mb-1">Newest data</p>
              <p className="text-sm font-medium text-[var(--text)]">
                {stats.newest_span_at ? formatDate(stats.newest_span_at) : "N/A"}
              </p>
            </div>
          </div>
          {stats.stats_approximate && (
            <p className="text-xs text-[var(--text-muted)]">Byte estimates are approximate.</p>
          )}
        </div>
      )}
    </div>
  );
}

// ── Retention Policy Card ─────────────────────────────────────────────────────

interface RetentionCardProps {
  projectId: string;
  apiKey: string;
  adminKey: string | null;
}

function RetentionCard({ projectId, apiKey, adminKey }: RetentionCardProps) {
  const { addToast } = useToast();
  const [selectedDays, setSelectedDays] = useState<number | null>(null);

  const { data: retention, isLoading } = useQuery<RetentionConfig>({
    queryKey: ["storageRetention", projectId],
    queryFn: () => storageApi.getRetention(projectId, apiKey),
  });

  const currentDays = retention?.retention_days ?? null;
  const isDirty = selectedDays !== null && selectedDays !== currentDays;

  const mutation = useMutation({
    mutationFn: (days: number) =>
      storageApi.updateRetention(projectId, days, apiKey, adminKey ?? ""),
    onSuccess: (updated) => {
      setSelectedDays(null);
      addToast({
        title: "Retention updated",
        message: `Data retention set to ${updated.retention_days} days.`,
        variant: "info",
      });
    },
    onError: (err: Error) => {
      addToast({ title: "Update failed", message: err.message, variant: "halt" });
    },
  });

  const displayDays = selectedDays ?? currentDays ?? 30;

  return (
    <div className={cardClass}>
      <p className="text-sm font-semibold text-[var(--text)]">Data Retention</p>

      {!adminKey && (
        <div className="border border-amber-700/50 bg-amber-950/20 rounded-lg px-3 py-2 text-xs text-amber-300/90">
          Admin key required to change retention settings.
        </div>
      )}

      {isLoading ? (
        <p className="text-sm text-[var(--text-muted)]">Loading…</p>
      ) : (
        <div className="flex flex-col gap-3">
          <div className="flex items-center gap-3">
            <label htmlFor="retention-days" className="text-sm text-[var(--text-muted)] shrink-0">
              Keep data for
            </label>
            <select
              id="retention-days"
              value={displayDays}
              onChange={(e) => setSelectedDays(Number(e.target.value))}
              disabled={!adminKey || mutation.isPending}
              className={`${inputClass} disabled:opacity-50`}
            >
              {RETENTION_OPTIONS.map((days) => (
                <option key={days} value={days}>
                  {days} days
                </option>
              ))}
            </select>
            <button
              onClick={() => {
                if (selectedDays !== null) mutation.mutate(selectedDays);
              }}
              disabled={!isDirty || !adminKey || mutation.isPending}
              className="px-4 py-2 text-sm rounded-lg bg-indigo-600 hover:bg-indigo-700 disabled:opacity-40 disabled:cursor-not-allowed text-white font-medium transition-colors"
            >
              {mutation.isPending ? "Saving…" : "Save"}
            </button>
          </div>

          {retention?.updated_at && (
            <p className="text-xs text-[var(--text-muted)]">
              Last updated: {formatDate(retention.updated_at)}
            </p>
          )}
        </div>
      )}
    </div>
  );
}

// ── Manual Purge Card ─────────────────────────────────────────────────────────

interface PurgeAgeFormProps {
  projectId: string;
  apiKey: string;
  adminKey: string | null;
}

function PurgeAgeForm({ projectId, apiKey, adminKey }: PurgeAgeFormProps) {
  const [days, setDays] = useState(30);
  const [includeEvals, setIncludeEvals] = useState(false);
  const [preview, setPreview] = useState<DryRunResult | null>(null);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [jobId, setJobId] = useState<string | null>(null);
  const [purgeError, setPurgeError] = useState<string | null>(null);
  const [purgeLoading, setPurgeLoading] = useState(false);

  async function handlePreview() {
    setPreviewLoading(true);
    setPreviewError(null);
    setPreview(null);
    try {
      const result = await storageApi.purgeByAge(
        projectId,
        days,
        includeEvals,
        true,
        apiKey,
        adminKey ?? ""
      );
      if ("spans_to_delete" in result) {
        setPreview(result);
      }
    } catch (err: unknown) {
      setPreviewError(err instanceof Error ? err.message : "Preview failed");
    } finally {
      setPreviewLoading(false);
    }
  }

  async function handlePurge() {
    setConfirmOpen(false);
    setPurgeLoading(true);
    setPurgeError(null);
    setJobId(null);
    try {
      const result = await storageApi.purgeByAge(
        projectId,
        days,
        includeEvals,
        false,
        apiKey,
        adminKey ?? ""
      );
      if ("job_id" in result) {
        setJobId(result.job_id);
        setPreview(null);
      }
    } catch (err: unknown) {
      setPurgeError(err instanceof Error ? err.message : "Purge failed");
    } finally {
      setPurgeLoading(false);
    }
  }

  return (
    <div className="flex flex-col gap-3">
      <p className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wider">
        Purge by Age
      </p>

      <div className="flex items-center gap-2 flex-wrap">
        <label className="text-sm text-[var(--text-muted)]">Purge data older than</label>
        <input
          type="number"
          min={7}
          value={days}
          onChange={(e) => {
            setDays(Number(e.target.value));
            setPreview(null);
          }}
          className={`${inputClass} w-24`}
        />
        <span className="text-sm text-[var(--text-muted)]">days</span>
      </div>

      <label className="flex items-center gap-2 text-sm text-[var(--text-muted)] cursor-pointer select-none">
        <input
          type="checkbox"
          checked={includeEvals}
          onChange={(e) => {
            setIncludeEvals(e.target.checked);
            setPreview(null);
          }}
          className="rounded border-[var(--border)]"
        />
        Also delete eval scores (cannot be undone)
      </label>

      {preview && (
        <p className="text-sm text-[var(--text)]">
          This will delete approximately{" "}
          <span className="font-semibold">{preview.spans_to_delete.toLocaleString()}</span> spans.
        </p>
      )}

      {previewError && <p className="text-sm text-red-400">{previewError}</p>}
      {purgeError && <p className="text-sm text-red-400">{purgeError}</p>}

      <div className="flex gap-2 flex-wrap">
        <button
          onClick={() => void handlePreview()}
          disabled={previewLoading || purgeLoading}
          className="px-3 py-1.5 text-sm rounded-lg border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] transition-colors disabled:opacity-50"
        >
          {previewLoading ? "Loading…" : "Preview"}
        </button>
        <button
          onClick={() => setConfirmOpen(true)}
          disabled={!preview || !adminKey || purgeLoading || previewLoading}
          className="px-3 py-1.5 text-sm rounded-lg bg-red-600 hover:bg-red-700 disabled:opacity-40 disabled:cursor-not-allowed text-white font-medium transition-colors"
        >
          {purgeLoading ? "Purging…" : "Purge"}
        </button>
      </div>

      <ConfirmModal
        isOpen={confirmOpen}
        title="Confirm Purge"
        message={`Are you sure? This will permanently delete ${preview ? preview.spans_to_delete.toLocaleString() : "N/A"} spans older than ${days} days. This cannot be undone.`}
        confirmLabel="Delete"
        onConfirm={() => void handlePurge()}
        onCancel={() => setConfirmOpen(false)}
      />

      {jobId && <JobStatusCard projectId={projectId} apiKey={apiKey} jobId={jobId} />}
    </div>
  );
}

interface PurgeRunFormProps {
  projectId: string;
  apiKey: string;
  adminKey: string | null;
}

function PurgeRunForm({ projectId, apiKey, adminKey }: PurgeRunFormProps) {
  const [runId, setRunId] = useState("");
  const [includeEvals, setIncludeEvals] = useState(false);
  const [preview, setPreview] = useState<DryRunResult | null>(null);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [jobId, setJobId] = useState<string | null>(null);
  const [purgeError, setPurgeError] = useState<string | null>(null);
  const [purgeLoading, setPurgeLoading] = useState(false);

  async function handlePreview() {
    if (!runId.trim()) return;
    setPreviewLoading(true);
    setPreviewError(null);
    setPreview(null);
    try {
      const result = await storageApi.purgeRun(
        projectId,
        runId.trim(),
        includeEvals,
        true,
        apiKey,
        adminKey ?? ""
      );
      if ("spans_to_delete" in result) {
        setPreview(result);
      }
    } catch (err: unknown) {
      setPreviewError(err instanceof Error ? err.message : "Preview failed");
    } finally {
      setPreviewLoading(false);
    }
  }

  async function handlePurge() {
    setConfirmOpen(false);
    if (!runId.trim()) return;
    setPurgeLoading(true);
    setPurgeError(null);
    setJobId(null);
    try {
      const result = await storageApi.purgeRun(
        projectId,
        runId.trim(),
        includeEvals,
        false,
        apiKey,
        adminKey ?? ""
      );
      if ("job_id" in result) {
        setJobId(result.job_id);
        setPreview(null);
      }
    } catch (err: unknown) {
      setPurgeError(err instanceof Error ? err.message : "Purge failed");
    } finally {
      setPurgeLoading(false);
    }
  }

  return (
    <div className="flex flex-col gap-3">
      <p className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wider">
        Purge by Run ID
      </p>

      <div className="flex flex-col gap-1">
        <label htmlFor="purge-run-id" className="text-sm text-[var(--text-muted)]">
          Run ID
        </label>
        <input
          id="purge-run-id"
          type="text"
          value={runId}
          onChange={(e) => {
            setRunId(e.target.value);
            setPreview(null);
          }}
          placeholder="e.g. 01JXXXXXXXXXXXXXXXXXXXXX"
          className={`${inputClass} w-full max-w-sm`}
        />
      </div>

      <label className="flex items-center gap-2 text-sm text-[var(--text-muted)] cursor-pointer select-none">
        <input
          type="checkbox"
          checked={includeEvals}
          onChange={(e) => {
            setIncludeEvals(e.target.checked);
            setPreview(null);
          }}
          className="rounded border-[var(--border)]"
        />
        Also delete eval scores (cannot be undone)
      </label>

      {preview && (
        <p className="text-sm text-[var(--text)]">
          This will delete approximately{" "}
          <span className="font-semibold">{preview.spans_to_delete.toLocaleString()}</span> spans.
        </p>
      )}

      {previewError && <p className="text-sm text-red-400">{previewError}</p>}
      {purgeError && <p className="text-sm text-red-400">{purgeError}</p>}

      <div className="flex gap-2 flex-wrap">
        <button
          onClick={() => void handlePreview()}
          disabled={!runId.trim() || previewLoading || purgeLoading}
          className="px-3 py-1.5 text-sm rounded-lg border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] transition-colors disabled:opacity-50"
        >
          {previewLoading ? "Loading…" : "Preview"}
        </button>
        <button
          onClick={() => setConfirmOpen(true)}
          disabled={!preview || !adminKey || purgeLoading || previewLoading}
          className="px-3 py-1.5 text-sm rounded-lg bg-red-600 hover:bg-red-700 disabled:opacity-40 disabled:cursor-not-allowed text-white font-medium transition-colors"
        >
          {purgeLoading ? "Purging…" : "Purge"}
        </button>
      </div>

      {!adminKey && (
        <p className="text-xs text-amber-300/90">Admin key required to purge data.</p>
      )}

      <ConfirmModal
        isOpen={confirmOpen}
        title="Confirm Run Purge"
        message={`Are you sure? This will permanently delete ${preview ? preview.spans_to_delete.toLocaleString() : "N/A"} spans for run ${runId}. This cannot be undone.`}
        confirmLabel="Delete"
        onConfirm={() => void handlePurge()}
        onCancel={() => setConfirmOpen(false)}
      />

      {jobId && <JobStatusCard projectId={projectId} apiKey={apiKey} jobId={jobId} />}
    </div>
  );
}

function ManualPurgeCard({ projectId, apiKey, adminKey }: RetentionCardProps) {
  return (
    <div className={cardClass}>
      <p className="text-sm font-semibold text-[var(--text)]">Manual Purge</p>
      <div className="border-t border-[var(--border)] pt-4">
        <PurgeAgeForm projectId={projectId} apiKey={apiKey} adminKey={adminKey} />
      </div>
      <div className="border-t border-[var(--border)] pt-4">
        <PurgeRunForm projectId={projectId} apiKey={apiKey} adminKey={adminKey} />
      </div>
    </div>
  );
}

// ── StorageSection ────────────────────────────────────────────────────────────

interface StorageSectionProps {
  projectId: string;
  apiKey: string;
  adminKey: string | null;
}

export function StorageSection({ projectId, apiKey, adminKey }: StorageSectionProps) {
  return (
    <div className="flex flex-col gap-6">
      <StorageUsageCard projectId={projectId} apiKey={apiKey} />
      <RetentionCard projectId={projectId} apiKey={apiKey} adminKey={adminKey} />
      <ManualPurgeCard projectId={projectId} apiKey={apiKey} adminKey={adminKey} />
    </div>
  );
}
