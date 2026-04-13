"use client";

import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { runTagsApi } from "@/lib/api";

const TAG_REGEX = /^[a-zA-Z0-9_:\-.]+$/;
const TAG_MAX_LEN = 64;

interface RunTagsProps {
  runId: string;
  projectId: string;
  initialTags: string[];
  readonly?: boolean;
}

export function RunTags({ runId, projectId, initialTags, readonly }: RunTagsProps) {
  const queryClient = useQueryClient();
  const [tags, setTags] = useState<string[]>(initialTags);
  const [inputValue, setInputValue] = useState("");
  const [inputError, setInputError] = useState<string | null>(null);

  const addMutation = useMutation({
    mutationFn: (tag: string) => runTagsApi.addTag(runId, tag, projectId),
    onSuccess: (_data, tag) => {
      setTags((prev) => [...prev, tag]);
      setInputValue("");
      setInputError(null);
      queryClient.invalidateQueries({ queryKey: ["run", runId] });
      queryClient.invalidateQueries({ queryKey: ["runs", projectId] });
    },
    onError: (err: Error) => {
      setInputError(err.message);
    },
  });

  const removeMutation = useMutation({
    mutationFn: (tag: string) => runTagsApi.removeTag(runId, tag, projectId),
    onSuccess: (_data, tag) => {
      setTags((prev) => prev.filter((t) => t !== tag));
      queryClient.invalidateQueries({ queryKey: ["run", runId] });
      queryClient.invalidateQueries({ queryKey: ["runs", projectId] });
    },
  });

  function validateTag(value: string): string | null {
    if (!value) return "Tag cannot be empty.";
    if (value.length > TAG_MAX_LEN) return `Tag must be ${TAG_MAX_LEN} characters or fewer.`;
    if (!TAG_REGEX.test(value)) return "Tag may only contain letters, numbers, _ : - .";
    if (tags.includes(value)) return "Tag already added.";
    return null;
  }

  function handleAdd() {
    const trimmed = inputValue.trim();
    const err = validateTag(trimmed);
    if (err) {
      setInputError(err);
      return;
    }
    setInputError(null);
    addMutation.mutate(trimmed);
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === "Enter") {
      e.preventDefault();
      handleAdd();
    }
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap gap-1.5">
        {tags.length === 0 && readonly && (
          <span className="text-xs text-[var(--text-muted)]">No tags.</span>
        )}
        {tags.map((tag) => (
          <span
            key={tag}
            className="inline-flex items-center gap-1 px-2 py-1 rounded-md text-xs font-medium bg-indigo-950/50 border border-indigo-700/50 text-indigo-300"
          >
            {tag}
            {!readonly && (
              <button
                type="button"
                aria-label={`Remove tag ${tag}`}
                onClick={() => removeMutation.mutate(tag)}
                disabled={removeMutation.isPending}
                className="ml-0.5 text-indigo-400 hover:text-red-400 transition-colors disabled:opacity-40"
              >
                ×
              </button>
            )}
          </span>
        ))}
      </div>

      {!readonly && (
        <div className="flex items-center gap-2">
          <input
            type="text"
            value={inputValue}
            onChange={(e) => {
              setInputValue(e.target.value);
              setInputError(null);
            }}
            onKeyDown={handleKeyDown}
            placeholder="Add tag…"
            maxLength={TAG_MAX_LEN}
            className="flex-1 text-sm rounded-lg border border-[var(--border)] bg-[var(--surface)] text-[var(--text)] placeholder:text-[var(--text-muted)] px-3 py-1.5 focus:outline-none focus:border-indigo-600"
          />
          <button
            type="button"
            onClick={handleAdd}
            disabled={addMutation.isPending}
            className="px-3 py-1.5 rounded-lg text-sm border border-indigo-600/60 text-indigo-300 hover:bg-indigo-600/10 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {addMutation.isPending ? "Adding…" : "Add"}
          </button>
        </div>
      )}

      {inputError && (
        <p className="text-xs text-red-400">{inputError}</p>
      )}
    </div>
  );
}
