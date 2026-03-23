"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { sessionsApi } from "@/lib/api";
import type { Session } from "@/lib/types";
import { formatCost } from "@/components/runs/RunRow";

interface Props {
  projectId: string;
}

function formatRelativeTime(dateStr: string): string {
  const date = new Date(dateStr);
  const diffMs = Date.now() - date.getTime();
  const diffMins = Math.floor(diffMs / 60_000);
  if (diffMins < 1) return "just now";
  if (diffMins < 60) return `${diffMins}m ago`;
  const diffHours = Math.floor(diffMins / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  const diffDays = Math.floor(diffHours / 24);
  return `${diffDays}d ago`;
}

function EmptyState() {
  return (
    <div className="rounded-xl border border-[var(--border)] bg-[var(--surface)] p-8 text-center">
      <p className="text-[var(--text)] font-medium mb-1">No sessions yet</p>
      <p className="text-sm text-[var(--text-muted)] mb-4">
        Sessions group multiple runs into a conversation. Call{" "}
        <code className="text-indigo-400">set_session_id()</code> in your SDK to get started:
      </p>
      <pre className="text-left inline-block rounded-lg border border-[var(--border)] bg-[#0d0d14] text-xs text-indigo-300 px-4 py-3">
        <code>{`from agentpulse import set_session_id, generate_session_id
set_session_id(generate_session_id())`}</code>
      </pre>
    </div>
  );
}

function SessionRow({ session, projectId }: { session: Session; projectId: string }) {
  const short =
    session.SessionID.length > 16
      ? session.SessionID.slice(0, 16) + "…"
      : session.SessionID;

  return (
    <Link
      href={`/projects/${projectId}/sessions/${encodeURIComponent(session.SessionID)}`}
      className="flex items-center gap-4 px-5 py-4 border border-[var(--border)] bg-[var(--surface)] rounded-xl hover:border-indigo-600 transition-colors group"
    >
      <div className="flex-1 min-w-0">
        <p
          className="text-sm font-mono text-indigo-300 truncate"
          title={session.SessionID}
        >
          ⬡ {short}
        </p>
        <p className="text-xs text-[var(--text-muted)]">
          First: {new Date(session.FirstRunAt).toLocaleString()}
        </p>
      </div>
      <div className="flex items-center gap-6 text-sm tabular-nums text-[var(--text-muted)]">
        <span title="Run count">
          <span className="text-[var(--text)]">{session.RunCount}</span> runs
        </span>
        <span className="text-indigo-400">{formatCost(session.TotalCostUSD)}</span>
        <span>{session.TotalTokens.toLocaleString()} tok</span>
        <span title={new Date(session.LastRunAt).toLocaleString()}>
          {formatRelativeTime(session.LastRunAt)}
        </span>
      </div>
    </Link>
  );
}

export function SessionList({ projectId }: Props) {
  const { data, isLoading } = useQuery({
    queryKey: ["sessions", projectId],
    queryFn: () => sessionsApi.list(projectId),
  });

  const sessions = data?.sessions ?? [];

  if (isLoading) {
    return (
      <div className="text-sm text-[var(--text-muted)] py-8 text-center">
        Loading sessions…
      </div>
    );
  }

  if (sessions.length === 0) {
    return <EmptyState />;
  }

  return (
    <div className="flex flex-col gap-3">
      {sessions.map((s) => (
        <SessionRow key={s.SessionID} session={s} projectId={projectId} />
      ))}
    </div>
  );
}
