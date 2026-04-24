"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Activity } from "lucide-react";
import { projectsApi } from "@/lib/api";
import { Navbar } from "@/components/Navbar";
import { CreateProjectModal } from "@/components/projects/CreateProjectModal";
import { ProjectCard } from "@/components/projects/ProjectCard";

export default function ProjectsPage() {
  const [showCreate, setShowCreate] = useState(false);
  const { data: projects, isLoading, error } = useQuery({
    queryKey: ["projects"],
    queryFn: projectsApi.list,
  });

  return (
    <div className="flex flex-col min-h-screen">
      <Navbar />
      <main className="flex-1 max-w-4xl mx-auto w-full px-6 py-10">
        <div className="flex items-center justify-between mb-8">
          <h1 className="text-2xl font-bold text-[var(--text)]">Projects</h1>
          <button
            onClick={() => setShowCreate(true)}
            className="bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium px-4 py-2 rounded-lg transition-colors"
          >
            + New Project
          </button>
        </div>

        {isLoading && (
          <div className="text-[var(--text-muted)]">Loading projects...</div>
        )}

        {error && (
          <div className="text-red-400 bg-red-950/40 border border-red-800 rounded-xl px-4 py-3">
            {(error as Error).message}
          </div>
        )}

        {projects && projects.length === 0 && (
          <div className="flex flex-col items-center text-center border border-[var(--border)] rounded-xl bg-[var(--surface)] px-8 py-16 gap-6">
            <div className="flex items-center justify-center w-16 h-16 rounded-2xl bg-[var(--surface-2)] border border-[var(--border)]">
              <Activity className="w-8 h-8 text-indigo-400" />
            </div>

            <div className="flex flex-col gap-2">
              <h2 className="text-2xl font-bold text-[var(--text)]">Welcome to AgentPulse</h2>
              <p className="text-[var(--text-muted)] text-base">Instrument your first AI agent in minutes.</p>
            </div>

            <div className="flex flex-col sm:flex-row items-start sm:items-center gap-4 sm:gap-0 w-full max-w-2xl">
              {[
                { step: "1", label: "Create a project", detail: "get your API key" },
                { step: "2", label: "Install the SDK", detail: "pip install agentpulse-sdk" },
                { step: "3", label: "Instrument your agent", detail: "traces appear here" },
              ].map(({ step, label, detail }, i, arr) => (
                <div key={step} className="flex sm:flex-1 items-center gap-3 sm:gap-0">
                  <div className="flex flex-col items-center sm:flex-1 gap-1">
                    <div className="flex items-center justify-center w-8 h-8 rounded-full bg-indigo-600 text-white text-xs font-bold shrink-0">
                      {step}
                    </div>
                    <p className="text-sm font-medium text-[var(--text)]">{label}</p>
                    <p className="text-xs text-[var(--text-muted)] font-mono">{detail}</p>
                  </div>
                  {i < arr.length - 1 && (
                    <div className="hidden sm:block h-px flex-1 bg-[var(--border)] mx-2" />
                  )}
                </div>
              ))}
            </div>

            <button
              onClick={() => setShowCreate(true)}
              className="bg-indigo-600 hover:bg-indigo-500 text-white font-medium px-6 py-2.5 rounded-lg transition-colors"
            >
              Create first project
            </button>
          </div>
        )}

        <div className="grid gap-4">
          {projects?.map((p) => (
            <ProjectCard key={p.ID} project={p} />
          ))}
        </div>
      </main>

      {showCreate && <CreateProjectModal onClose={() => setShowCreate(false)} />}
    </div>
  );
}
