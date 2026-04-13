"use client";

import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { runTagsApi } from "@/lib/api";

const NOTE_MAX_LEN = 5000;

interface RunAnnotationProps {
  runId: string;
  projectId: string;
  initialNote?: string | null;
  readonly?: boolean;
}

export function RunAnnotation({ runId, projectId, initialNote, readonly }: RunAnnotationProps) {
  const queryClient = useQueryClient();
  const [note, setNote] = useState(initialNote ?? "");
  const [savedNote, setSavedNote] = useState(initialNote ?? "");
  const [saveStatus, setSaveStatus] = useState<"idle" | "saving" | "saved" | "error">("idle");
  const [saveError, setSaveError] = useState<string | null>(null);

  const upsertMutation = useMutation({
    mutationFn: (text: string) => runTagsApi.upsertAnnotation(runId, text, projectId),
    onSuccess: () => {
      setSavedNote(note);
      setSaveStatus("saved");
      setSaveError(null);
      queryClient.invalidateQueries({ queryKey: ["run", runId] });
      setTimeout(() => setSaveStatus("idle"), 2000);
    },
    onError: (err: Error) => {
      setSaveStatus("error");
      setSaveError(err.message);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: () => runTagsApi.deleteAnnotation(runId, projectId),
    onSuccess: () => {
      setNote("");
      setSavedNote("");
      setSaveStatus("idle");
      setSaveError(null);
      queryClient.invalidateQueries({ queryKey: ["run", runId] });
    },
    onError: (err: Error) => {
      setSaveStatus("error");
      setSaveError(err.message);
    },
  });

  function handleSave() {
    setSaveStatus("saving");
    upsertMutation.mutate(note);
  }

  const isDirty = note !== savedNote;

  if (readonly) {
    return (
      <div className="text-sm text-[var(--text)] whitespace-pre-wrap rounded-lg border border-[var(--border)] bg-[var(--surface)] px-3 py-2 min-h-[60px]">
        {savedNote || <span className="text-[var(--text-muted)]">No annotation.</span>}
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-2">
      <textarea
        value={note}
        onChange={(e) => {
          setNote(e.target.value);
          setSaveStatus("idle");
          setSaveError(null);
        }}
        maxLength={NOTE_MAX_LEN}
        rows={4}
        placeholder="Add a note about this run…"
        className="w-full text-sm rounded-lg border border-[var(--border)] bg-[var(--surface)] text-[var(--text)] placeholder:text-[var(--text-muted)] px-3 py-2 focus:outline-none focus:border-indigo-600 resize-y"
      />

      <div className="flex items-center justify-between gap-3">
        <span className="text-xs text-[var(--text-muted)] tabular-nums">
          {note.length} / {NOTE_MAX_LEN}
        </span>

        <div className="flex items-center gap-2">
          {saveStatus === "saved" && (
            <span className="text-xs text-green-400">Saved</span>
          )}
          {saveStatus === "error" && saveError && (
            <span className="text-xs text-red-400">{saveError}</span>
          )}

          {savedNote && (
            <button
              type="button"
              onClick={() => deleteMutation.mutate()}
              disabled={deleteMutation.isPending || upsertMutation.isPending}
              className="px-3 py-1.5 rounded-lg text-sm border border-[var(--border)] text-[var(--text-muted)] hover:border-red-600 hover:text-red-400 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {deleteMutation.isPending ? "Clearing…" : "Clear"}
            </button>
          )}

          <button
            type="button"
            onClick={handleSave}
            disabled={!isDirty || upsertMutation.isPending || !note.trim()}
            className="px-3 py-1.5 rounded-lg text-sm border border-indigo-600/60 text-indigo-300 hover:bg-indigo-600/10 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {upsertMutation.isPending ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}
