"use client";
import Link from "next/link";

export function SessionBadge({ projectId, sessionId }: { projectId: string; sessionId: string }) {
  const short = sessionId.length > 8 ? sessionId.slice(0, 8) + "…" : sessionId;
  return (
    <Link
      href={`/projects/${projectId}/sessions/${encodeURIComponent(sessionId)}`}
      onClick={(e) => e.stopPropagation()}
      title={`Session: ${sessionId}`}
      className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-indigo-950/60 border border-indigo-700/60 text-indigo-300 hover:border-indigo-500 transition-colors"
    >
      ⬡ {short}
    </Link>
  );
}
