"use client";

import { useState, useEffect } from "react";

const SPAN_KIND_OPTIONS = [
  { value: "", label: "All Types" },
  { value: "llm.call", label: "LLM Call" },
  { value: "tool.call", label: "Tool Call" },
  { value: "agent.handoff", label: "Agent Handoff" },
  { value: "memory.read", label: "Memory Read" },
  { value: "memory.write", label: "Memory Write" },
];

interface Props {
  onSearch: (query: string, spanKind: string) => void;
  isLoading?: boolean;
}

export function SearchBar({ onSearch, isLoading = false }: Props) {
  const [inputValue, setInputValue] = useState("");
  const [spanKind, setSpanKind] = useState("");
  const [debouncedValue, setDebouncedValue] = useState("");

  // Debounce input value by 300ms.
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedValue(inputValue);
    }, 300);
    return () => clearTimeout(timer);
  }, [inputValue]);

  // Fire onSearch when debounced value or spanKind changes.
  useEffect(() => {
    if (debouncedValue.length === 0) {
      onSearch("", spanKind);
    } else if (debouncedValue.length >= 3) {
      onSearch(debouncedValue, spanKind);
    }
  }, [debouncedValue, spanKind]); // eslint-disable-line react-hooks/exhaustive-deps

  function handleClear() {
    setInputValue("");
    setDebouncedValue("");
    onSearch("", spanKind);
  }

  const showHelperText = inputValue.length > 0 && inputValue.length < 3;

  return (
    <div className="mb-6">
      <div className="flex gap-2">
        {/* Search input */}
        <div className="relative flex-1">
          {/* Search icon */}
          <svg
            className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-[var(--text-muted)] pointer-events-none"
            xmlns="http://www.w3.org/2000/svg"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>

          <input
            type="text"
            value={inputValue}
            onChange={(e) => setInputValue(e.target.value)}
            placeholder="Search prompts, completions, tool I/O..."
            className="w-full pl-10 pr-10 py-2.5 bg-[var(--surface)] border border-[var(--border)] rounded-lg text-sm text-[var(--text)] placeholder-[var(--text-muted)] focus:outline-none focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500"
          />

          {/* Loading spinner or clear button */}
          {isLoading && inputValue.length >= 3 ? (
            <svg
              className="absolute right-3 top-1/2 -translate-y-1/2 w-4 h-4 text-indigo-400 animate-spin"
              xmlns="http://www.w3.org/2000/svg"
              fill="none"
              viewBox="0 0 24 24"
            >
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
            </svg>
          ) : inputValue.length > 0 ? (
            <button
              onClick={handleClear}
              className="absolute right-3 top-1/2 -translate-y-1/2 text-[var(--text-muted)] hover:text-[var(--text)] transition-colors"
              aria-label="Clear search"
            >
              <svg className="w-4 h-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          ) : null}
        </div>

        {/* Span kind filter */}
        <select
          value={spanKind}
          onChange={(e) => setSpanKind(e.target.value)}
          className="px-3 py-2.5 bg-[var(--surface)] border border-[var(--border)] rounded-lg text-sm text-[var(--text)] focus:outline-none focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500"
        >
          {SPAN_KIND_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
      </div>

      {showHelperText && (
        <p className="mt-1.5 text-xs text-[var(--text-muted)]">
          Type at least 3 characters to search
        </p>
      )}
    </div>
  );
}
