"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { settingsApi } from "@/lib/api";
import { getApiKey } from "@/lib/api-keys";
import { useToast } from "@/components/toast/ToastContext";
import { StorageSection } from "@/components/settings/StorageSection";
import { IngestTokensSection } from "@/components/settings/IngestTokensSection";
import { LoopDetectionSection } from "@/components/settings/LoopDetectionSection";
import type { PIICustomRule } from "@/lib/types";

interface Props {
  projectId: string;
}

const BUILTIN_PATTERNS = [
  "credit_card",
  "ssn",
  "email",
  "jwt",
  "api_key_openai",
  "api_key_anthropic",
  "bearer_inline",
  "aws_access_key",
  "github_pat",
  "stripe_live",
  "google_api_key",
  "pem_header",
  "slack_token",
  "phone_us",
] as const;

const MAX_CUSTOM_RULES = 20;

function validatePattern(pattern: string): string | null {
  if (!pattern.trim()) return "Pattern is required.";
  try {
    const re = new RegExp(pattern);
    if (re.test("")) return "Pattern is too broad — it matches the empty string.";
    return null;
  } catch {
    return "Invalid regular expression.";
  }
}

interface AddRuleFormProps {
  onAdd: (rule: PIICustomRule) => void;
  onCancel: () => void;
  isPending: boolean;
}

function AddRuleForm({ onAdd, onCancel, isPending }: AddRuleFormProps) {
  const [ruleName, setRuleName] = useState("");
  const [pattern, setPattern] = useState("");
  const [patternError, setPatternError] = useState<string | null>(null);

  function handlePatternChange(value: string) {
    setPattern(value);
    if (value.trim()) {
      setPatternError(validatePattern(value));
    } else {
      setPatternError(null);
    }
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const err = validatePattern(pattern);
    if (err) { setPatternError(err); return; }
    if (!ruleName.trim()) return;
    onAdd({ name: ruleName.trim(), pattern: pattern.trim(), enabled: true });
  }

  const inputClass =
    "w-full rounded-lg border border-[var(--border)] bg-[var(--surface)] text-[var(--text)] px-3 py-2 text-sm focus:outline-none focus:border-indigo-500";

  return (
    <form onSubmit={handleSubmit} className="border border-[var(--border)] rounded-xl p-4 mt-3 bg-[var(--surface-2)]">
      <p className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wider mb-3">New Custom Rule</p>
      <div className="flex flex-col gap-3">
        <div>
          <label className="block text-xs text-[var(--text-muted)] mb-1">Name</label>
          <input
            autoFocus
            type="text"
            required
            placeholder="e.g. internal-token"
            value={ruleName}
            onChange={(e) => setRuleName(e.target.value)}
            className={inputClass}
          />
        </div>
        <div>
          <label className="block text-xs text-[var(--text-muted)] mb-1">Pattern (regex)</label>
          <input
            type="text"
            required
            placeholder="e.g. sk-int-[A-Za-z0-9]{32}"
            value={pattern}
            onChange={(e) => handlePatternChange(e.target.value)}
            className={`${inputClass}${patternError ? " border-red-500" : ""}`}
          />
          {patternError && (
            <p className="text-xs text-red-400 mt-1">{patternError}</p>
          )}
        </div>
        <div className="flex gap-2 justify-end">
          <button
            type="button"
            onClick={onCancel}
            className="px-3 py-1.5 text-xs rounded-lg border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] transition-colors"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={isPending || !!patternError || !ruleName.trim() || !pattern.trim()}
            className="px-3 py-1.5 text-xs rounded-lg bg-indigo-600 hover:bg-indigo-700 disabled:opacity-50 text-white font-medium transition-colors"
          >
            {isPending ? "Saving..." : "Add Rule"}
          </button>
        </div>
      </div>
    </form>
  );
}

type SettingsTab = "security" | "storage" | "tokens" | "loops";

const TAB_LABELS: Record<SettingsTab, string> = {
  security: "Security",
  storage: "Storage",
  tokens: "Tokens",
  loops: "Loop Detection",
};

function SecurityContent({ projectId }: Props) {
  const queryClient = useQueryClient();
  const { addToast } = useToast();
  const [showAddForm, setShowAddForm] = useState(false);

  const adminKey =
    typeof window !== "undefined"
      ? localStorage.getItem(`adminKey_${projectId}`)
      : null;

  const hasAdminKey = Boolean(adminKey);

  const { data: config, isLoading } = useQuery({
    queryKey: ["piiConfig", projectId],
    queryFn: () => settingsApi.get(projectId),
  });

  const mutation = useMutation({
    mutationFn: (body: { pii_redaction_enabled: boolean; pii_custom_rules: PIICustomRule[] }) => {
      const key = localStorage.getItem(`adminKey_${projectId}`) ?? "";
      return settingsApi.update(projectId, body, key);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["piiConfig", projectId] });
    },
    onError: (err: Error) => {
      addToast({ title: "Settings error", message: err.message, variant: "halt" });
    },
  });

  function handleToggleRedaction() {
    if (!config) return;
    mutation.mutate({
      pii_redaction_enabled: !config.pii_redaction_enabled,
      pii_custom_rules: config.pii_custom_rules ?? [],
    });
  }

  function handleToggleRule(index: number) {
    if (!config) return;
    const updated = config.pii_custom_rules.map((r, i) =>
      i === index ? { ...r, enabled: !r.enabled } : r
    );
    mutation.mutate({
      pii_redaction_enabled: config.pii_redaction_enabled,
      pii_custom_rules: updated,
    });
  }

  function handleDeleteRule(index: number) {
    if (!config) return;
    if (!confirm("Delete this custom rule?")) return;
    const updated = config.pii_custom_rules.filter((_, i) => i !== index);
    mutation.mutate({
      pii_redaction_enabled: config.pii_redaction_enabled,
      pii_custom_rules: updated,
    });
  }

  function handleAddRule(rule: PIICustomRule) {
    if (!config) return;
    const updated = [...(config.pii_custom_rules ?? []), rule];
    mutation.mutate(
      { pii_redaction_enabled: config.pii_redaction_enabled, pii_custom_rules: updated },
      { onSuccess: () => setShowAddForm(false) }
    );
  }

  const customRules = config?.pii_custom_rules ?? [];
  const atLimit = customRules.length >= MAX_CUSTOM_RULES;
  const isEnabled = config?.pii_redaction_enabled ?? false;
  const isMutating = mutation.isPending;
  const readOnly = !hasAdminKey;

  if (isLoading) {
    return (
      <p className="text-sm text-[var(--text-muted)] py-6">Loading settings...</p>
    );
  }

  return (
    <div className="flex flex-col gap-8">
      {/* No admin key notice */}
      {readOnly && (
        <div className="border border-amber-700/50 bg-amber-950/20 rounded-xl px-4 py-3 text-sm text-amber-300/90">
          Settings changes require your Admin Key. Your Admin Key was shown once when this project was created. If you&apos;ve lost it, you&apos;ll need to recreate the project.
        </div>
      )}

      {/* PII Redaction toggle */}
      <div>
        <div className="flex items-start justify-between gap-4">
          <div className="flex-1">
            <p className="text-sm font-semibold text-[var(--text)]">PII Redaction</p>
            <p className="text-xs text-[var(--text-muted)] mt-1">
              Automatically mask sensitive data (emails, API keys, credit cards) in span attributes before storage. Changes take effect within 30 seconds.
            </p>
          </div>
          {/* Toggle switch */}
          <button
            role="switch"
            aria-checked={isEnabled}
            aria-label="PII Redaction"
            disabled={readOnly || isMutating}
            onClick={handleToggleRedaction}
            className={`relative shrink-0 inline-flex h-6 w-11 items-center rounded-full transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-indigo-500 disabled:opacity-40 disabled:cursor-not-allowed ${
              isEnabled ? "bg-indigo-600" : "bg-[var(--border)]"
            }`}
          >
            <span
              className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                isEnabled ? "translate-x-6" : "translate-x-1"
              }`}
            />
          </button>
        </div>

        {/* Warning box */}
        <div className="mt-3 border border-amber-700/50 bg-amber-950/20 rounded-xl px-4 py-3 text-xs text-amber-300/90">
          Redaction is irreversible. Once enabled, matched data in new spans is permanently replaced with <span className="font-mono">[REDACTED:type]</span>. Existing stored spans are not affected.
        </div>
      </div>

      {/* Built-in patterns */}
      <div>
        <p className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wider mb-3">
          Built-in Patterns (always active when redaction is enabled)
        </p>
        <div className="flex flex-wrap gap-2">
          {BUILTIN_PATTERNS.map((name) => (
            <span
              key={name}
              className="inline-block text-xs font-mono px-2 py-1 rounded-md bg-[var(--surface-2)] border border-[var(--border)] text-[var(--text-muted)]"
            >
              {name}
            </span>
          ))}
        </div>
      </div>

      {/* Custom rules */}
      <div>
        <div className="flex items-center justify-between mb-3">
          <p className="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wider">Custom Rules</p>
          <div className="flex items-center gap-2">
            {atLimit && (
              <span className="text-xs text-amber-400">Maximum 20 custom rules</span>
            )}
            <button
              onClick={() => setShowAddForm(true)}
              disabled={readOnly || isMutating || atLimit || showAddForm}
              className="text-xs px-3 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white font-medium transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            >
              + Add Rule
            </button>
          </div>
        </div>

        {customRules.length === 0 && !showAddForm ? (
          <p className="text-xs text-[var(--text-muted)] border border-[var(--border)] rounded-xl px-4 py-6 text-center">
            No custom rules yet. Add a rule to redact project-specific patterns.
          </p>
        ) : (
          <div className="border border-[var(--border)] rounded-xl overflow-hidden">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-[var(--border)] bg-[var(--surface-2)]">
                  <th className="text-left px-4 py-2.5 text-xs font-medium text-[var(--text-muted)]">Name</th>
                  <th className="text-left px-4 py-2.5 text-xs font-medium text-[var(--text-muted)]">Pattern</th>
                  <th className="text-left px-4 py-2.5 text-xs font-medium text-[var(--text-muted)]">Status</th>
                  <th className="px-4 py-2.5" />
                </tr>
              </thead>
              <tbody>
                {customRules.map((rule, index) => (
                  <tr key={`${rule.name}-${index}`} className="border-t border-[var(--border)] first:border-0">
                    <td className="px-4 py-3 text-[var(--text)] font-medium text-sm">{rule.name}</td>
                    <td className="px-4 py-3">
                      <span className="text-xs font-mono text-[var(--text-muted)] break-all">{rule.pattern}</span>
                    </td>
                    <td className="px-4 py-3">
                      <span className={`text-xs font-medium ${rule.enabled ? "text-green-400" : "text-[var(--text-muted)]"}`}>
                        {rule.enabled ? "Active" : "Inactive"}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex items-center justify-end gap-2">
                        <button
                          onClick={() => handleToggleRule(index)}
                          disabled={readOnly || isMutating}
                          className="text-xs px-2.5 py-1 rounded bg-[var(--surface-2)] hover:bg-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] transition-colors disabled:opacity-40"
                        >
                          {rule.enabled ? "Disable" : "Enable"}
                        </button>
                        <button
                          onClick={() => handleDeleteRule(index)}
                          disabled={readOnly || isMutating}
                          className="text-xs px-2.5 py-1 rounded bg-red-950/30 hover:bg-red-950/50 text-red-400 transition-colors disabled:opacity-40"
                        >
                          Delete
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {showAddForm && (
          <AddRuleForm
            onAdd={handleAddRule}
            onCancel={() => setShowAddForm(false)}
            isPending={isMutating}
          />
        )}
      </div>
    </div>
  );
}

export function SettingsSection({ projectId }: Props) {
  const [activeTab, setActiveTab] = useState<SettingsTab>("security");

  const adminKey =
    typeof window !== "undefined"
      ? localStorage.getItem(`adminKey_${projectId}`)
      : null;

  const apiKey = getApiKey(projectId) ?? "";

  const TABS: SettingsTab[] = ["security", "storage", "tokens", "loops"];

  return (
    <div className="flex flex-col gap-6">
      {/* Tab bar */}
      <div className="flex gap-1 border-b border-[var(--border)]">
        {TABS.map((tab) => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            className={`px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px ${
              activeTab === tab
                ? "border-indigo-500 text-[var(--text)]"
                : "border-transparent text-[var(--text-muted)] hover:text-[var(--text)]"
            }`}
          >
            {TAB_LABELS[tab]}
          </button>
        ))}
      </div>

      {/* Tab content */}
      {activeTab === "security" && <SecurityContent projectId={projectId} />}
      {activeTab === "storage" && (
        <StorageSection
          projectId={projectId}
          apiKey={apiKey}
          adminKey={adminKey}
        />
      )}
      {activeTab === "tokens" && <IngestTokensSection projectId={projectId} />}
      {activeTab === "loops" && <LoopDetectionSection projectId={projectId} />}
    </div>
  );
}
