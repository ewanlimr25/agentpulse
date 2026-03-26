"use client";

export type DeltaFormat = "currency" | "number" | "percentage" | "ms";

interface DeltaIndicatorProps {
  valueA: number;
  valueB: number;
  format?: DeltaFormat;
  /** When true, a decrease is considered "better" (e.g. cost, errors). */
  lowerIsBetter?: boolean;
}

function formatValue(value: number, format: DeltaFormat): string {
  switch (format) {
    case "currency":
      return `$${Math.abs(value).toFixed(4)}`;
    case "percentage":
      return `${Math.abs(value).toFixed(1)}%`;
    case "ms":
      return `${Math.abs(value).toFixed(0)}ms`;
    default:
      return Math.abs(value).toLocaleString();
  }
}

export function DeltaIndicator({
  valueA,
  valueB,
  format = "number",
  lowerIsBetter = false,
}: DeltaIndicatorProps) {
  const delta = valueB - valueA;

  if (delta === 0 || (valueA === 0 && valueB === 0)) {
    return <span className="text-[var(--text-muted)]">—</span>;
  }

  const pct =
    valueA !== 0 ? Math.abs((delta / valueA) * 100).toFixed(1) : null;

  // Determine direction: positive delta = B > A
  const isIncrease = delta > 0;

  // "better" = improvement direction
  const isBetter = lowerIsBetter ? !isIncrease : isIncrease;
  const colorClass = isBetter ? "text-green-400" : "text-red-400";

  const arrow = isIncrease ? "↑" : "↓";
  const sign = isIncrease ? "+" : "−";

  return (
    <span className={`flex items-center gap-0.5 text-xs font-medium ${colorClass}`}>
      <span>{arrow}</span>
      <span>
        {sign}{formatValue(Math.abs(delta), format)}
        {pct !== null && ` (${pct}%)`}
      </span>
    </span>
  );
}
