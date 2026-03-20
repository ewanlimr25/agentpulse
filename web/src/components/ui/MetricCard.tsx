interface Props {
  label: string;
  value: string | number;
  sub?: string;
  accent?: boolean;
}

export function MetricCard({ label, value, sub, accent }: Props) {
  return (
    <div
      className={`rounded-xl border p-4 flex flex-col gap-1 ${
        accent
          ? "border-indigo-700 bg-indigo-950/40"
          : "border-[var(--border)] bg-[var(--surface)]"
      }`}
    >
      <p className="text-xs text-[var(--text-muted)] uppercase tracking-wide">{label}</p>
      <p className={`text-2xl font-semibold tabular-nums ${accent ? "text-indigo-300" : "text-[var(--text)]"}`}>
        {value}
      </p>
      {sub && <p className="text-xs text-[var(--text-muted)]">{sub}</p>}
    </div>
  );
}
