"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
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
          <div className="text-[var(--text-muted)] border border-[var(--border)] rounded-xl px-6 py-10 text-center">
            No projects yet. Click <strong className="text-[var(--text)]">+ New Project</strong> to get started.
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
