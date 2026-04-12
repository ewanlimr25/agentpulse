"use client";

import { useEffect, useRef, useState } from "react";
import { formatCost } from "./RunRow";

interface LiveCostCounterProps {
  costUSD: number;
  isLive: boolean;
}

export function LiveCostCounter({ costUSD, isLive }: LiveCostCounterProps) {
  const [displayCost, setDisplayCost] = useState(costUSD);
  const prevCostRef = useRef(costUSD);

  useEffect(() => {
    if (costUSD !== prevCostRef.current) {
      prevCostRef.current = costUSD;
      setDisplayCost(costUSD);
    }
  }, [costUSD]);

  return (
    <span
      className={`tabular-nums transition-colors duration-300 ${
        isLive ? "text-green-400" : "text-[var(--text)]"
      }`}
    >
      {formatCost(displayCost)}
    </span>
  );
}
