"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { projectsApi } from "@/lib/api";
import { Navbar } from "@/components/Navbar";

export default function ProjectsPage() {
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
            No projects yet. Create one via the API:
            <pre className="mt-4 text-xs bg-[var(--surface-2)] rounded-lg p-4 text-left text-green-400 overflow-x-auto">
              {`curl -X POST http://localhost:8080/api/v1/projects \\
  -H "Content-Type: application/json" \\
  -d '{"name":"my-project"}'`}
            </pre>
          </div>
        )}

        <div className="grid gap-4">
          {projects?.map((p) => (
            <Link
              key={p.ID}
              href={`/projects/${p.ID}`}
              className="block border border-[var(--border)] bg-[var(--surface)] rounded-xl px-6 py-4 hover:border-indigo-600 transition-colors group"
            >
              <div className="flex items-center justify-between">
                <span className="font-semibold text-[var(--text)] group-hover:text-indigo-300 transition-colors">
                  {p.Name}
                </span>
                <span className="text-xs text-[var(--text-muted)]">
                  {new Date(p.CreatedAt).toLocaleDateString()}
                </span>
              </div>
              <p className="text-xs text-[var(--text-muted)] mt-1 font-mono">{p.ID}</p>
            </Link>
          ))}
        </div>
      </main>
    </div>
  );
}
