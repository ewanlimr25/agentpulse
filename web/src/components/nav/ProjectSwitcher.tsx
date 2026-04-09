"use client";

import { useState, useRef, useEffect } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { ChevronDown, Plus } from "lucide-react";
import { projectsApi } from "@/lib/api";
import { CreateProjectModal } from "@/components/projects/CreateProjectModal";

interface Props {
  projectId: string;
  collapsed?: boolean;
}

/** First 1–2 uppercase letters used as avatar initials. */
function initials(name: string): string {
  const words = name.trim().split(/[\s-_]+/);
  if (words.length >= 2) return (words[0][0] + words[1][0]).toUpperCase();
  return name.slice(0, 2).toUpperCase();
}

export function ProjectSwitcher({ projectId, collapsed = false }: Props) {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  const { data: projects } = useQuery({
    queryKey: ["projects"],
    queryFn: projectsApi.list,
  });

  const current = projects?.find((p) => p.ID === projectId);

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, []);

  const dropdown = open && (
    <div
      className={`absolute bg-[var(--surface)] border border-[var(--border)] rounded-lg shadow-lg z-50 overflow-hidden w-56
        ${collapsed ? "left-full top-0 ml-2" : "left-0 right-0 mt-1 top-full"}`}
    >
      {projects?.map((p) => (
        <button
          key={p.ID}
          onClick={() => {
            setOpen(false);
            if (p.ID !== projectId) router.push(`/projects/${p.ID}/overview`);
          }}
          className={`w-full text-left px-3 py-2.5 text-sm transition-colors ${
            p.ID === projectId
              ? "bg-indigo-600/20 text-indigo-300"
              : "text-[var(--text)] hover:bg-[var(--surface-2)]"
          }`}
        >
          <div className="font-medium truncate">{p.Name}</div>
          <div className="text-xs text-[var(--text-muted)] font-mono truncate">{p.ID}</div>
        </button>
      ))}

      <div className="border-t border-[var(--border)]">
        <button
          onClick={() => { setOpen(false); setShowCreate(true); }}
          className="w-full flex items-center gap-2 px-3 py-2.5 text-sm text-[var(--text-muted)] hover:text-[var(--text)] hover:bg-[var(--surface-2)] transition-colors"
        >
          <Plus className="w-4 h-4" />
          New project
        </button>
      </div>
    </div>
  );

  return (
    <>
      <div ref={ref} className="relative">
        {collapsed ? (
          /* Avatar button in collapsed sidebar mode */
          <button
            onClick={() => setOpen((v) => !v)}
            className="w-full flex items-center justify-center"
            title={current?.Name ?? projectId}
          >
            <span className="w-8 h-8 rounded-md bg-indigo-600/30 border border-indigo-500/40 flex items-center justify-center text-xs font-bold text-indigo-300 hover:bg-indigo-600/50 transition-colors">
              {initials(current?.Name ?? projectId)}
            </span>
          </button>
        ) : (
          /* Full button in expanded mode */
          <button
            onClick={() => setOpen((v) => !v)}
            className="w-full flex items-center justify-between gap-2 px-3 py-2 text-sm text-[var(--text)] bg-[var(--surface-2)] border border-[var(--border)] rounded-lg hover:border-indigo-500 transition-colors"
          >
            <span className="truncate font-medium">{current?.Name ?? projectId}</span>
            <ChevronDown className="w-4 h-4 shrink-0 text-[var(--text-muted)]" />
          </button>
        )}

        {dropdown}
      </div>

      {showCreate && <CreateProjectModal onClose={() => setShowCreate(false)} />}
    </>
  );
}
