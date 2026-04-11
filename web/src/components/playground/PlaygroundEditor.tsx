"use client";

import type { PlaygroundVariant, PlaygroundMessage, ModelInfo } from "@/lib/types";
import { ModelPicker } from "./ModelPicker";

interface PlaygroundEditorProps {
  variant: PlaygroundVariant;
  models: ModelInfo[];
  onUpdate: (changes: Partial<PlaygroundVariant>) => void;
  onRun: () => void;
  isRunning: boolean;
}

export function PlaygroundEditor({
  variant,
  models,
  onUpdate,
  onRun,
  isRunning,
}: PlaygroundEditorProps) {
  const messages = variant.Messages ?? [];

  function handleMessageChange(
    index: number,
    field: keyof PlaygroundMessage,
    value: string
  ) {
    const updated: PlaygroundMessage[] = messages.map((msg, i) =>
      i === index ? { ...msg, [field]: value } : msg
    );
    onUpdate({ Messages: updated });
  }

  function handleAddMessage() {
    const updated: PlaygroundMessage[] = [
      ...messages,
      { role: "user", content: "" },
    ];
    onUpdate({ Messages: updated });
  }

  function handleRemoveMessage(index: number) {
    const updated: PlaygroundMessage[] = messages.filter((_, i) => i !== index);
    onUpdate({ Messages: updated });
  }

  return (
    <div className="space-y-4">
      {/* Label */}
      <div>
        <label className="block text-xs text-[var(--text-muted)] mb-1">
          Label
        </label>
        <input
          type="text"
          value={variant.Label}
          onChange={(e) => onUpdate({ Label: e.target.value })}
          className="w-full rounded-lg bg-[var(--surface-2)] text-[var(--text)] border border-[var(--border)] px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-blue-500"
        />
      </div>

      {/* Model */}
      <div>
        <label className="block text-xs text-[var(--text-muted)] mb-1">
          Model
        </label>
        <ModelPicker
          models={models}
          value={variant.ModelID}
          onChange={(modelId) => onUpdate({ ModelID: modelId })}
        />
      </div>

      {/* System prompt */}
      <div>
        <label className="block text-xs text-[var(--text-muted)] mb-1">
          System prompt
        </label>
        <textarea
          value={variant.System}
          onChange={(e) => onUpdate({ System: e.target.value })}
          rows={3}
          className="w-full rounded-lg bg-[var(--surface-2)] text-[var(--text)] border border-[var(--border)] px-3 py-2 text-sm font-mono resize-y focus:outline-none focus:ring-1 focus:ring-blue-500"
          placeholder="You are a helpful assistant..."
        />
      </div>

      {/* Messages */}
      <div>
        <label className="block text-xs text-[var(--text-muted)] mb-1">
          Messages
        </label>
        <div className="space-y-2">
          {messages.map((msg, i) => (
            <div
              key={i}
              className="flex gap-2 items-start rounded-lg bg-[var(--surface-2)] border border-[var(--border)] p-2"
            >
              <select
                value={msg.role}
                onChange={(e) =>
                  handleMessageChange(
                    i,
                    "role",
                    e.target.value as PlaygroundMessage["role"]
                  )
                }
                className="rounded bg-[var(--surface-1)] text-[var(--text)] border border-[var(--border)] px-2 py-1 text-xs focus:outline-none"
              >
                <option value="user">user</option>
                <option value="assistant">assistant</option>
              </select>
              <textarea
                value={msg.content}
                onChange={(e) =>
                  handleMessageChange(i, "content", e.target.value)
                }
                rows={2}
                className="flex-1 rounded bg-[var(--surface-1)] text-[var(--text)] border border-[var(--border)] px-2 py-1 text-sm font-mono resize-y focus:outline-none focus:ring-1 focus:ring-blue-500"
                placeholder="Message content..."
              />
              <button
                type="button"
                onClick={() => handleRemoveMessage(i)}
                className="text-[var(--text-muted)] hover:text-red-400 text-xs px-1 py-1 shrink-0"
                title="Remove message"
              >
                &times;
              </button>
            </div>
          ))}
          <button
            type="button"
            onClick={handleAddMessage}
            className="text-xs text-blue-400 hover:text-blue-300"
          >
            + Add message
          </button>
        </div>
      </div>

      {/* Temperature */}
      <div>
        <label className="block text-xs text-[var(--text-muted)] mb-1">
          Temperature: {variant.Temperature ?? 1}
        </label>
        <input
          type="range"
          min={0}
          max={2}
          step={0.1}
          value={variant.Temperature ?? 1}
          onChange={(e) =>
            onUpdate({ Temperature: parseFloat(e.target.value) })
          }
          className="w-full accent-blue-500"
        />
      </div>

      {/* Max tokens */}
      <div>
        <label className="block text-xs text-[var(--text-muted)] mb-1">
          Max tokens
        </label>
        <input
          type="number"
          min={1}
          max={4096}
          value={variant.MaxTokens ?? ""}
          onChange={(e) => {
            const v = e.target.value;
            onUpdate({ MaxTokens: v === "" ? null : parseInt(v, 10) });
          }}
          placeholder="Default"
          className="w-full rounded-lg bg-[var(--surface-2)] text-[var(--text)] border border-[var(--border)] px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-blue-500"
        />
      </div>

      {/* Run button */}
      <button
        type="button"
        onClick={onRun}
        disabled={isRunning}
        className="w-full rounded-lg bg-blue-600 hover:bg-blue-500 disabled:bg-blue-600/50 disabled:cursor-not-allowed text-white text-sm font-medium py-2 flex items-center justify-center gap-2 transition-colors"
      >
        {isRunning ? (
          <>
            <svg
              className="animate-spin h-4 w-4"
              viewBox="0 0 24 24"
              fill="none"
            >
              <circle
                className="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                strokeWidth="4"
              />
              <path
                className="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
              />
            </svg>
            Running...
          </>
        ) : (
          "Run"
        )}
      </button>
    </div>
  );
}
