"use client";

import React from "react";

interface ChartCardProps {
  title: string;
  children: React.ReactNode;
  isEmpty?: boolean;
  emptyMessage?: string;
}

export function ChartCard({
  title,
  children,
  isEmpty = false,
  emptyMessage = "Not enough data",
}: ChartCardProps) {
  return (
    <div className="rounded-xl border border-[var(--border)] bg-[var(--surface)] p-4">
      <h3 className="text-sm font-semibold text-[var(--text-muted)] uppercase tracking-wide mb-3">
        {title}
      </h3>
      {isEmpty ? (
        <div className="h-64 flex items-center justify-center">
          <p className="text-[var(--text-muted)] text-sm">{emptyMessage}</p>
        </div>
      ) : (
        <div className="h-64">{children}</div>
      )}
    </div>
  );
}
