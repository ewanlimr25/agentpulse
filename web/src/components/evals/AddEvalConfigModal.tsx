"use client";

import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { evalsApi } from "@/lib/api";

const AVAILABLE_JUDGE_MODELS = [
  { id: "claude-haiku-4-5", label: "claude-haiku-4-5" },
  { id: "gpt-4o-mini", label: "gpt-4o-mini" },
  { id: "gemini-2.0-flash", label: "gemini-2.0-flash" },
] as const;

interface Props {
  projectId: string;
  onClose: () => void;
}

export function AddEvalConfigModal({ projectId, onClose }: Props) {
  const queryClient = useQueryClient();
  const [evalName, setEvalName] = useState("");
  const [spanKind, setSpanKind] = useState<"llm.call" | "tool.call">("llm.call");
  const [promptTemplate, setPromptTemplate] = useState("");
  const [agentFilter, setAgentFilter] = useState(""); // comma-separated agent names
  const [judgeModels, setJudgeModels] = useState<string[]>(["claude-haiku-4-5"]);
  const [error, setError] = useState("");

  function toggleJudgeModel(modelId: string) {
    setJudgeModels((prev) => {
      if (prev.includes(modelId)) {
        // Always keep at least one model selected
        if (prev.length === 1) return prev;
        return prev.filter((m) => m !== modelId);
      }
      return [...prev, modelId];
    });
  }

  const mutation = useMutation({
    mutationFn: () => {
      const agents = agentFilter.split(",").map((s) => s.trim()).filter(Boolean);
      return evalsApi.upsertConfig(projectId, {
        eval_name: `custom:${evalName.trim().toLowerCase().replace(/\s+/g, "_")}`,
        enabled: true,
        span_kind: spanKind,
        prompt_template: promptTemplate.trim(),
        ...(agents.length > 0 ? { scope_filter: { agent_name: agents } } : {}),
        judge_models: judgeModels,
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["evalConfigs", projectId] });
      onClose();
    },
    onError: (err: Error) => {
      setError(err.message);
    },
  });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    if (!evalName.trim()) { setError("Name is required"); return; }
    if (!promptTemplate.trim()) { setError("Prompt template is required"); return; }
    if (promptTemplate.length > 4000) { setError("Prompt template must be 4000 characters or fewer"); return; }
    if (!promptTemplate.includes("{{input}}") && !promptTemplate.includes("{{output}}")) {
      setError("Prompt template must contain {{input}} or {{output}}");
      return;
    }
    mutation.mutate();
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 px-4">
      <div className="bg-[var(--surface)] border border-[var(--border)] rounded-2xl w-full max-w-lg shadow-xl">
        <div className="flex items-center justify-between px-6 py-4 border-b border-[var(--border)]">
          <h2 className="text-base font-semibold text-[var(--text)]">Create Custom Eval</h2>
          <button onClick={onClose} className="text-[var(--text-muted)] hover:text-[var(--text)] text-xl leading-none">×</button>
        </div>

        <form onSubmit={handleSubmit} className="px-6 py-5 flex flex-col gap-4">
          <div>
            <label className="block text-xs font-medium text-[var(--text-muted)] mb-1.5">Name</label>
            <input
              type="text"
              value={evalName}
              onChange={(e) => setEvalName(e.target.value)}
              placeholder="brand_voice"
              className="w-full bg-[var(--surface-2)] border border-[var(--border)] rounded-lg px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-indigo-500"
            />
            <p className="text-xs text-[var(--text-muted)] mt-1">Will be prefixed with <code className="font-mono">custom:</code></p>
          </div>

          <div>
            <label className="block text-xs font-medium text-[var(--text-muted)] mb-1.5">Applies to</label>
            <div className="flex gap-4">
              {(["llm.call", "tool.call"] as const).map((kind) => (
                <label key={kind} className="flex items-center gap-2 text-sm text-[var(--text)] cursor-pointer">
                  <input
                    type="radio"
                    checked={spanKind === kind}
                    onChange={() => setSpanKind(kind)}
                    className="accent-indigo-500"
                  />
                  <code className="font-mono text-xs">{kind}</code>
                </label>
              ))}
            </div>
          </div>

          <div>
            <label className="block text-xs font-medium text-[var(--text-muted)] mb-1.5">Agent Filter <span className="font-normal">(optional)</span></label>
            <input
              type="text"
              value={agentFilter}
              onChange={(e) => setAgentFilter(e.target.value)}
              placeholder="researcher, writer"
              className="w-full bg-[var(--surface-2)] border border-[var(--border)] rounded-lg px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-indigo-500"
            />
            <p className="text-xs text-[var(--text-muted)] mt-1">Comma-separated agent names. Leave blank to run on all agents.</p>
          </div>

          <div>
            <label className="block text-xs font-medium text-[var(--text-muted)] mb-1.5">Judge Models</label>
            <div className="flex flex-col gap-2">
              {AVAILABLE_JUDGE_MODELS.map((model) => (
                <label key={model.id} className="flex items-center gap-2.5 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={judgeModels.includes(model.id)}
                    onChange={() => toggleJudgeModel(model.id)}
                    className="accent-indigo-500 w-3.5 h-3.5"
                  />
                  <span className="text-sm font-mono text-[var(--text)]">{model.label}</span>
                  {model.id === "claude-haiku-4-5" && (
                    <span className="text-[10px] px-1.5 py-0.5 rounded bg-indigo-950/40 border border-indigo-700/50 text-indigo-400">default</span>
                  )}
                </label>
              ))}
            </div>
            <p className="text-xs text-[var(--text-muted)] mt-1.5">Multiple models increase eval cost proportionally.</p>
          </div>

          <div>
            <label className="block text-xs font-medium text-[var(--text-muted)] mb-1.5">Judge Prompt Template</label>
            <textarea
              value={promptTemplate}
              onChange={(e) => setPromptTemplate(e.target.value)}
              rows={8}
              placeholder={`You are evaluating whether the response follows brand voice guidelines.\n\nInput: {{input}}\nOutput: {{output}}\n\nScore 0.0 (off-brand) to 1.0 (perfect).`}
              className="w-full bg-[var(--surface-2)] border border-[var(--border)] rounded-lg px-3 py-2 text-sm font-mono text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-indigo-500 resize-none"
            />
            <p className="text-xs text-[var(--text-muted)] mt-1">
              Variables: <code className="font-mono">{"{{input}}"}</code>, <code className="font-mono">{"{{output}}"}</code>, <code className="font-mono">{"{{context}}"}</code>, <code className="font-mono">{"{{tool_name}}"}</code>
            </p>
          </div>

          {error && <p className="text-xs text-red-400">{error}</p>}

          <div className="flex justify-end gap-3 pt-1">
            <button type="button" onClick={onClose} className="px-4 py-2 text-sm text-[var(--text-muted)] hover:text-[var(--text)] transition-colors">
              Cancel
            </button>
            <button
              type="submit"
              disabled={mutation.isPending}
              className="px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white text-sm font-medium transition-colors disabled:opacity-50"
            >
              {mutation.isPending ? "Creating…" : "Create & Activate"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
