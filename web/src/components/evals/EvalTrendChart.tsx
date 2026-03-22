"use client";

import { useState } from "react";
import { LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer, ReferenceLine } from "recharts";
import type { QualityPoint } from "@/lib/chart-utils";
import { ChartCard } from "@/components/charts/ChartCard";

const EVAL_COLORS: Record<string, string> = {
  relevance:        "#34d399",  // emerald
  hallucination:    "#fb7185",  // rose
  faithfulness:     "#fbbf24",  // amber
  toxicity:         "#a78bfa",  // violet
  tool_correctness: "#2dd4bf",  // teal
};
const DEFAULT_COLOR = "#60a5fa"; // blue for custom types

function evalColor(name: string): string {
  return EVAL_COLORS[name] ?? DEFAULT_COLOR;
}

interface Props {
  data: QualityPoint[];
  /** If true, show all individual eval lines by default (Evals tab). Otherwise show composite avg. */
  defaultShowAll?: boolean;
}

export function EvalTrendChart({ data, defaultShowAll = false }: Props) {
  // Collect all eval type names present in the data.
  const evalNames = Array.from(
    new Set(data.flatMap((p) => Object.keys(p.byEvalName ?? {})))
  );

  // Which lines are currently toggled on.
  const [activeLines, setActiveLines] = useState<Set<string>>(() => {
    if (defaultShowAll && evalNames.length > 0) {
      return new Set(evalNames.slice(0, 3));
    }
    return new Set<string>();
  });

  const showComposite = activeLines.size === 0;

  function toggle(name: string) {
    setActiveLines((prev) => {
      const next = new Set(prev);
      if (next.has(name)) {
        next.delete(name);
      } else if (next.size < 3) {
        next.add(name);
      }
      return next;
    });
  }

  const xAxisInterval = data.length > 15 ? Math.floor(data.length / 8) : undefined;

  return (
    <ChartCard title="Quality Score" isEmpty={data.length < 2}>
      {/* Toggle pills — only shown when multiple eval types exist */}
      {evalNames.length > 1 && (
        <div className="flex flex-wrap gap-1.5 mb-3">
          <button
            onClick={() => setActiveLines(new Set())}
            className={`px-2 py-0.5 rounded text-xs font-medium transition-colors ${
              showComposite
                ? "bg-indigo-600 text-white"
                : "bg-[var(--surface-2)] text-[var(--text-muted)] hover:text-[var(--text)]"
            }`}
          >
            Avg
          </button>
          {evalNames.map((name) => (
            <button
              key={name}
              onClick={() => toggle(name)}
              disabled={!activeLines.has(name) && activeLines.size >= 3}
              className={`px-2 py-0.5 rounded text-xs font-medium transition-colors ${
                activeLines.has(name)
                  ? "text-white"
                  : "bg-[var(--surface-2)] text-[var(--text-muted)] hover:text-[var(--text)] disabled:opacity-40 disabled:cursor-not-allowed"
              }`}
              style={activeLines.has(name) ? { backgroundColor: evalColor(name) } : undefined}
            >
              {name.replace(/_/g, " ")}
            </button>
          ))}
        </div>
      )}

      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={data}>
          <XAxis
            dataKey="label"
            tick={{ fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            {...(xAxisInterval !== undefined ? { interval: xAxisInterval } : {})}
          />
          <YAxis
            domain={[0, 1]}
            tickFormatter={(v: number) => `${(v * 100).toFixed(0)}%`}
            tick={{ fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            width={44}
          />
          {/* eslint-disable-next-line @typescript-eslint/no-explicit-any */}
          <Tooltip formatter={(value: any, name: any) => [`${(Number(value) * 100).toFixed(1)}%`, String(name) === "score" ? "Avg Quality" : String(name).replace(/_/g, " ")]} />
          <ReferenceLine y={0.7} stroke="#6b7280" strokeDasharray="4 2" strokeOpacity={0.5} />

          {showComposite ? (
            <Line type="monotone" dataKey="score" stroke="#34d399" strokeWidth={2} dot={false} activeDot={{ r: 4 }} />
          ) : (
            Array.from(activeLines).map((name) => (
              <Line
                key={name}
                type="monotone"
                dataKey={`byEvalName.${name}`}
                name={name}
                stroke={evalColor(name)}
                strokeWidth={2}
                dot={false}
                activeDot={{ r: 4 }}
              />
            ))
          )}
        </LineChart>
      </ResponsiveContainer>
    </ChartCard>
  );
}
