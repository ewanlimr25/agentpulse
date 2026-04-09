"use client";

import { useEffect, useState } from "react";
import { ApiKeyPrompt } from "@/components/ApiKeyPrompt";
import { ProjectSidebar } from "@/components/nav/ProjectSidebar";
import { CommandPalette } from "@/components/nav/CommandPalette";
import { useProjectAuth } from "@/lib/hooks/useProjectAuth";

interface Props {
  projectId: string;
  children: React.ReactNode;
}

export function ProjectLayoutClient({ projectId, children }: Props) {
  const { gated, isValidating, keyError, submitKey } = useProjectAuth(projectId);
  const [paletteOpen, setPaletteOpen] = useState(false);

  // Global Cmd+K / Ctrl+K shortcut
  useEffect(() => {
    function handler(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key === "k") {
        e.preventDefault();
        setPaletteOpen(true);
      }
    }
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, []);

  if (gated || isValidating) {
    return (
      <div className="flex flex-col min-h-screen">
        <main className="flex-1 max-w-5xl mx-auto w-full px-6 py-10">
          <ApiKeyPrompt
            projectId={projectId}
            keyError={keyError}
            onKeySubmit={submitKey}
          />
        </main>
      </div>
    );
  }

  return (
    <div className="flex h-screen overflow-hidden">
      <ProjectSidebar
        projectId={projectId}
        onSearchOpen={() => setPaletteOpen(true)}
      />
      <main className="flex-1 h-full overflow-y-auto flex flex-col">
        {children}
      </main>

      <CommandPalette
        open={paletteOpen}
        onClose={() => setPaletteOpen(false)}
        projectId={projectId}
      />
    </div>
  );
}
