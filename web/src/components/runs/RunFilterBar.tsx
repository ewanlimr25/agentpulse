import { useState, useRef, useEffect } from "react";

export type StatusFilter = "all" | "ok" | "error";
export type SortBy = "newest" | "oldest" | "most-expensive" | "longest";

interface Props {
  statusFilter: StatusFilter;
  onStatusChange: (s: StatusFilter) => void;
  sortBy: SortBy;
  onSortChange: (s: SortBy) => void;
  availableTags?: string[];
  selectedTags?: string[];
  onTagsChange?: (tags: string[]) => void;
}

const statuses: { value: StatusFilter; label: string }[] = [
  { value: "all", label: "All" },
  { value: "ok", label: "OK" },
  { value: "error", label: "Error" },
];

export function RunFilterBar({
  statusFilter,
  onStatusChange,
  sortBy,
  onSortChange,
  availableTags = [],
  selectedTags = [],
  onTagsChange,
}: Props) {
  const [tagOpen, setTagOpen] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setTagOpen(false);
      }
    }
    if (tagOpen) document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [tagOpen]);

  function toggleTag(tag: string) {
    if (!onTagsChange) return;
    if (selectedTags.includes(tag)) {
      onTagsChange(selectedTags.filter((t) => t !== tag));
    } else {
      onTagsChange([...selectedTags, tag]);
    }
  }

  return (
    <div className="flex items-center gap-3 flex-wrap">
      {/* Status pills */}
      <div className="flex items-center rounded-lg border border-[var(--border)] overflow-hidden text-sm">
        {statuses.map(({ value, label }) => (
          <button
            key={value}
            type="button"
            onClick={() => onStatusChange(value)}
            className={`px-3 py-1.5 transition-colors ${
              statusFilter === value
                ? "bg-indigo-600 text-white"
                : "bg-[var(--surface)] text-[var(--text-muted)] hover:text-[var(--text)]"
            }`}
          >
            {label}
          </button>
        ))}
      </div>

      {/* Sort dropdown */}
      <select
        value={sortBy}
        onChange={(e) => onSortChange(e.target.value as SortBy)}
        className="text-sm rounded-lg border border-[var(--border)] bg-[var(--surface)] text-[var(--text-muted)] px-3 py-1.5 focus:outline-none focus:border-indigo-600"
      >
        <option value="newest">Newest first</option>
        <option value="oldest">Oldest first</option>
        <option value="most-expensive">Most expensive</option>
        <option value="longest">Longest</option>
      </select>

      {/* Tag multi-select (only rendered when tags are available or a handler is provided) */}
      {onTagsChange && (
        <div className="relative" ref={dropdownRef}>
          <button
            type="button"
            onClick={() => setTagOpen((prev) => !prev)}
            className={`flex items-center gap-1.5 text-sm rounded-lg border px-3 py-1.5 transition-colors focus:outline-none ${
              selectedTags.length > 0
                ? "border-indigo-500 bg-indigo-950/30 text-indigo-300"
                : "border-[var(--border)] bg-[var(--surface)] text-[var(--text-muted)] hover:text-[var(--text)]"
            }`}
          >
            <span>Tags</span>
            {selectedTags.length > 0 && (
              <span className="inline-flex items-center justify-center w-4 h-4 rounded-full bg-indigo-600 text-white text-[10px] font-bold">
                {selectedTags.length}
              </span>
            )}
            <svg className="w-3 h-3 opacity-60" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
            </svg>
          </button>

          {tagOpen && (
            <div className="absolute top-full left-0 mt-1 z-20 min-w-[180px] rounded-lg border border-[var(--border)] bg-[var(--surface)] shadow-lg py-1">
              {availableTags.length === 0 ? (
                <p className="px-3 py-2 text-xs text-[var(--text-muted)]">No tags yet.</p>
              ) : (
                availableTags.map((tag) => {
                  const active = selectedTags.includes(tag);
                  return (
                    <button
                      key={tag}
                      type="button"
                      onClick={() => toggleTag(tag)}
                      className={`w-full flex items-center gap-2 text-left px-3 py-1.5 text-sm transition-colors ${
                        active
                          ? "text-indigo-300 bg-indigo-950/30"
                          : "text-[var(--text-muted)] hover:text-[var(--text)] hover:bg-[var(--border)]/20"
                      }`}
                    >
                      <span className={`w-3.5 h-3.5 rounded-sm border flex items-center justify-center flex-shrink-0 ${
                        active ? "border-indigo-500 bg-indigo-600" : "border-[var(--border)]"
                      }`}>
                        {active && (
                          <svg className="w-2.5 h-2.5 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={3} d="M5 13l4 4L19 7" />
                          </svg>
                        )}
                      </span>
                      {tag}
                    </button>
                  );
                })
              )}
              {selectedTags.length > 0 && (
                <>
                  <div className="border-t border-[var(--border)] my-1" />
                  <button
                    type="button"
                    onClick={() => onTagsChange([])}
                    className="w-full text-left px-3 py-1.5 text-xs text-[var(--text-muted)] hover:text-[var(--text)] transition-colors"
                  >
                    Clear tag filter
                  </button>
                </>
              )}
            </div>
          )}
        </div>
      )}

      {/* Active tag pills (outside dropdown for visibility) */}
      {selectedTags.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {selectedTags.map((tag) => (
            <span
              key={tag}
              className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-xs font-medium bg-indigo-950/50 border border-indigo-700/50 text-indigo-300"
            >
              {tag}
              <button
                type="button"
                aria-label={`Remove tag filter ${tag}`}
                onClick={() => onTagsChange?.(selectedTags.filter((t) => t !== tag))}
                className="text-indigo-400 hover:text-red-400 transition-colors"
              >
                ×
              </button>
            </span>
          ))}
        </div>
      )}
    </div>
  );
}
