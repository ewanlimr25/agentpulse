export type StatusFilter = "all" | "ok" | "error";
export type SortBy = "newest" | "oldest" | "most-expensive" | "longest";

interface Props {
  statusFilter: StatusFilter;
  onStatusChange: (s: StatusFilter) => void;
  sortBy: SortBy;
  onSortChange: (s: SortBy) => void;
}

const statuses: { value: StatusFilter; label: string }[] = [
  { value: "all", label: "All" },
  { value: "ok", label: "OK" },
  { value: "error", label: "Error" },
];

export function RunFilterBar({ statusFilter, onStatusChange, sortBy, onSortChange }: Props) {
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
    </div>
  );
}
