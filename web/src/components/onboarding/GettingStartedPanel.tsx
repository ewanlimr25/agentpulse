"use client";

import { useState, useEffect } from "react";
import { FRAMEWORKS, DEFAULT_FRAMEWORK_INDEX } from "@/lib/onboarding-snippets";
import type { Framework } from "@/lib/onboarding-snippets";
import { getApiKey } from "@/lib/api-keys";
import { useProjectHealth } from "@/lib/hooks/useProjectHealth";
import { FrameworkSnippet } from "./FrameworkSnippet";
import { CollectorHealthBadge } from "./CollectorHealthBadge";

interface GettingStartedPanelProps {
  projectId: string;
  onDismiss: () => void;
}

type Step = 0 | 1 | 2 | 3;
const STEPS = ["Install", "Configure", "Instrument", "Verify"] as const;
const ENDPOINT_DEFAULT = "http://localhost:4318";

function applyPlaceholders(text: string, projectId: string, apiKey: string): string {
  return text
    .replace(/\{\{PROJECT_ID\}\}/g, projectId)
    .replace(/\{\{API_KEY\}\}/g, apiKey || "<your-api-key>")
    .replace(/\{\{INGEST_TOKEN\}\}/g, apiKey || "<your-ingest-token>")
    .replace(/\{\{ENDPOINT\}\}/g, ENDPOINT_DEFAULT);
}

const pythonFrameworks = FRAMEWORKS.filter((f) => f.language === "python");
const tsFrameworks = FRAMEWORKS.filter((f) => f.language === "typescript");
const shellFrameworks = FRAMEWORKS.filter((f) => f.language === "shell");

export function GettingStartedPanel({ projectId, onDismiss }: GettingStartedPanelProps) {
  const [step, setStep] = useState<Step>(0);
  const [successShown, setSuccessShown] = useState(false);

  // Persist framework selection
  const frameworkKey = `onboarding:framework:${projectId}`;
  function loadSavedFramework(): Framework {
    if (typeof window === "undefined") return FRAMEWORKS[DEFAULT_FRAMEWORK_INDEX];
    const saved = localStorage.getItem(frameworkKey);
    if (saved) {
      const found = FRAMEWORKS.find((f) => f.id === saved);
      if (found) return found;
    }
    return FRAMEWORKS[DEFAULT_FRAMEWORK_INDEX];
  }

  const [framework, setFramework] = useState<Framework>(loadSavedFramework);

  function selectFramework(fw: Framework) {
    setFramework(fw);
    if (typeof window !== "undefined") {
      localStorage.setItem(frameworkKey, fw.id);
    }
  }

  // Only poll health when on the Verify step
  const { isReceiving } = useProjectHealth(projectId, step === 3);

  // Auto-dismiss once data is flowing
  useEffect(() => {
    if (isReceiving && step === 3 && !successShown) {
      setSuccessShown(true);
      const timer = setTimeout(() => {
        handleDismiss();
      }, 3000);
      return () => clearTimeout(timer);
    }
  }, [isReceiving, step, successShown]);

  function handleDismiss() {
    if (typeof window !== "undefined") {
      localStorage.setItem(`onboarding:dismissed:${projectId}`, "1");
    }
    onDismiss();
  }

  const apiKey = getApiKey(projectId) ?? "";

  return (
    <div className="border border-[var(--border)] bg-[var(--surface)] rounded-2xl p-6">
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-base font-semibold text-[var(--text)]">Getting Started</h2>
        <button
          onClick={handleDismiss}
          className="text-xs text-[var(--text-muted)] hover:text-[var(--text)] transition-colors"
        >
          Dismiss
        </button>
      </div>

      {/* Step indicator */}
      <div className="flex items-center gap-2 mb-6">
        {STEPS.map((label, idx) => (
          <div key={label} className="flex items-center gap-2">
            <button
              onClick={() => setStep(idx as Step)}
              className={`flex items-center gap-1.5 text-xs font-medium transition-colors ${
                idx === step
                  ? "text-indigo-400"
                  : idx < step
                  ? "text-[var(--text-muted)]"
                  : "text-[var(--text-muted)] opacity-50"
              }`}
            >
              <span
                className={`w-5 h-5 rounded-full flex items-center justify-center text-xs font-semibold border ${
                  idx === step
                    ? "border-indigo-500 bg-indigo-600 text-white"
                    : idx < step
                    ? "border-[var(--border)] bg-[var(--surface)] text-[var(--text-muted)]"
                    : "border-[var(--border)] text-[var(--text-muted)] opacity-50"
                }`}
              >
                {idx + 1}
              </span>
              {label}
            </button>
            {idx < STEPS.length - 1 && (
              <span className="text-[var(--border)] text-xs">›</span>
            )}
          </div>
        ))}
      </div>

      {/* Step content */}
      <div className="min-h-[200px]">
        {step === 0 && (
          <StepInstall
            framework={framework}
            onSelect={selectFramework}
            projectId={projectId}
            apiKey={apiKey}
          />
        )}
        {step === 1 && (
          <StepConfigure
            framework={framework}
            projectId={projectId}
            apiKey={apiKey}
          />
        )}
        {step === 2 && (
          <StepInstrument
            framework={framework}
            projectId={projectId}
            apiKey={apiKey}
          />
        )}
        {step === 3 && (
          <StepVerify projectId={projectId} isReceiving={isReceiving} successShown={successShown} />
        )}
      </div>

      {/* Navigation */}
      <div className="flex items-center justify-between mt-6 pt-4 border-t border-[var(--border)]">
        <button
          onClick={() => setStep((s) => Math.max(0, s - 1) as Step)}
          disabled={step === 0}
          className="text-sm px-4 py-1.5 rounded-lg border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] hover:border-indigo-500 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
        >
          ← Back
        </button>
        {step < 3 ? (
          <button
            onClick={() => setStep((s) => Math.min(3, s + 1) as Step)}
            className="text-sm px-4 py-1.5 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white font-medium transition-colors"
          >
            Next →
          </button>
        ) : (
          <button
            onClick={handleDismiss}
            className="text-sm px-4 py-1.5 rounded-lg border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] transition-colors"
          >
            Close
          </button>
        )}
      </div>
    </div>
  );
}

// ── Sub-step components ───────────────────────────────────────────────────────

interface StepInstallProps {
  framework: Framework;
  onSelect: (fw: Framework) => void;
  projectId: string;
  apiKey: string;
}

function StepInstall({ framework, onSelect, projectId, apiKey }: StepInstallProps) {
  const installRendered = applyPlaceholders(framework.installCmd, projectId, apiKey);

  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm text-[var(--text-muted)]">
        Choose your framework and install the AgentPulse SDK.
      </p>

      <div className="flex flex-col gap-2">
        <p className="text-xs font-medium text-[var(--text-muted)] uppercase tracking-wide">Python</p>
        <div className="flex flex-wrap gap-2">
          {pythonFrameworks.map((fw) => (
            <FrameworkPill key={fw.id} fw={fw} selected={framework.id === fw.id} onSelect={onSelect} />
          ))}
        </div>
      </div>

      <div className="flex flex-col gap-2">
        <p className="text-xs font-medium text-[var(--text-muted)] uppercase tracking-wide">TypeScript</p>
        <div className="flex flex-wrap gap-2">
          {tsFrameworks.map((fw) => (
            <FrameworkPill key={fw.id} fw={fw} selected={framework.id === fw.id} onSelect={onSelect} />
          ))}
        </div>
      </div>

      <div className="flex flex-col gap-2">
        <p className="text-xs font-medium text-[var(--text-muted)] uppercase tracking-wide">CLI Tools</p>
        <div className="flex flex-wrap gap-2">
          {shellFrameworks.map((fw) => (
            <FrameworkPill key={fw.id} fw={fw} selected={framework.id === fw.id} onSelect={onSelect} />
          ))}
        </div>
      </div>

      <div className="mt-2">
        <p className="text-xs text-[var(--text-muted)] mb-2">Install command</p>
        <CopyBlock text={installRendered} />
      </div>
    </div>
  );
}

interface StepConfigureProps {
  framework: Framework;
  projectId: string;
  apiKey: string;
}

function StepConfigure({ framework, projectId, apiKey }: StepConfigureProps) {
  const envRendered = applyPlaceholders(framework.envVars, projectId, apiKey);

  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm text-[var(--text-muted)]">
        Set these environment variables before running your application.
      </p>
      <CopyBlock text={envRendered} />
      <div className="text-xs text-[var(--text-muted)] flex flex-col gap-1">
        <p><span className="font-mono text-[var(--text)]">AGENTPULSE_PROJECT_ID</span> — identifies which project spans are attributed to.</p>
        <p><span className="font-mono text-[var(--text)]">AGENTPULSE_API_KEY</span> — authenticates your SDK with the collector.</p>
        <p><span className="font-mono text-[var(--text)]">AGENTPULSE_ENDPOINT</span> — OTLP/HTTP endpoint for the local collector.</p>
      </div>
    </div>
  );
}

interface StepInstrumentProps {
  framework: Framework;
  projectId: string;
  apiKey: string;
}

function StepInstrument({ framework, projectId, apiKey }: StepInstrumentProps) {
  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm text-[var(--text-muted)]">
        Add the following snippet to your application to start emitting traces.
      </p>
      <FrameworkSnippet
        code={framework.code}
        projectId={projectId}
        apiKey={apiKey}
      />
    </div>
  );
}

interface StepVerifyProps {
  projectId: string;
  isReceiving: boolean;
  successShown: boolean;
}

function StepVerify({ projectId, isReceiving, successShown }: StepVerifyProps) {
  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm text-[var(--text-muted)]">
        Run your application. AgentPulse will detect the first trace automatically.
      </p>

      <div className="flex items-center gap-3 border border-[var(--border)] rounded-xl p-4">
        <CollectorHealthBadge projectId={projectId} enabled />
      </div>

      {successShown && isReceiving && (
        <div className="flex items-center gap-2 text-sm text-green-400 border border-green-800/50 bg-green-950/30 rounded-xl p-3">
          <span className="text-base">✓</span>
          First trace received! Closing in a moment…
        </div>
      )}

      {!isReceiving && (
        <p className="text-xs text-[var(--text-muted)]">
          Make sure your collector is running (<span className="font-mono">docker compose up -d</span>) and your environment variables are set correctly.
        </p>
      )}
    </div>
  );
}

// ── Shared primitives ─────────────────────────────────────────────────────────

function FrameworkPill({
  fw,
  selected,
  onSelect,
}: {
  fw: Framework;
  selected: boolean;
  onSelect: (fw: Framework) => void;
}) {
  return (
    <button
      onClick={() => onSelect(fw)}
      className={`text-xs px-3 py-1.5 rounded-lg border font-medium transition-colors ${
        selected
          ? "border-indigo-500 bg-indigo-600 text-white"
          : "border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] hover:border-indigo-500"
      }`}
    >
      {fw.label}
    </button>
  );
}

function CopyBlock({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);

  function handleCopy() {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  return (
    <div className="relative group">
      <pre className="bg-[var(--surface)] border border-[var(--border)] rounded-xl p-4 overflow-x-auto">
        <code className="font-mono text-sm text-[var(--text)]">{text}</code>
      </pre>
      <button
        onClick={handleCopy}
        className="absolute top-3 right-3 text-xs px-2 py-1 rounded-md bg-[var(--surface)] border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] hover:border-indigo-500 transition-colors"
      >
        {copied ? "Copied!" : "Copy"}
      </button>
    </div>
  );
}
