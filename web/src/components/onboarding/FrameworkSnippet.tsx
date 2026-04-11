"use client";

import { useState } from "react";

interface FrameworkSnippetProps {
  code: string;
  projectId: string;
  apiKey: string;
}

const ENDPOINT_DEFAULT = "http://localhost:4318";

function applyPlaceholders(code: string, projectId: string, apiKey: string): string {
  return code
    .replace(/\{\{PROJECT_ID\}\}/g, projectId)
    .replace(/\{\{API_KEY\}\}/g, apiKey || "<your-api-key>")
    .replace(/\{\{ENDPOINT\}\}/g, ENDPOINT_DEFAULT);
}

export function FrameworkSnippet({ code, projectId, apiKey }: FrameworkSnippetProps) {
  const [copied, setCopied] = useState(false);
  const rendered = applyPlaceholders(code, projectId, apiKey);

  function handleCopy() {
    navigator.clipboard.writeText(rendered).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  return (
    <div className="relative group">
      <pre className="bg-[var(--surface)] border border-[var(--border)] rounded-xl p-4 overflow-x-auto">
        <code className="font-mono text-sm text-[var(--text)]">{rendered}</code>
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
