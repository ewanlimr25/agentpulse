"use client";

import { useEffect, useRef } from "react";

import { budgetApi } from "@/lib/api";
import { useToast } from "@/components/toast/ToastContext";

const POLL_INTERVAL_MS = 5_000;

/**
 * Polls recent budget alerts across ALL projects every 5 s.
 * Shows a toast with project name, rule name, and a link to the budget page.
 * Mount once at the app root (Providers) so it works on any page.
 */
export function useGlobalAlertPoller() {
  const { addToast } = useToast();
  const addToastRef = useRef(addToast);
  useEffect(() => { addToastRef.current = addToast; }, [addToast]);

  const lastSeenRef = useRef<string | null>(null);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;

    const poll = async () => {
      if (!mountedRef.current) return;
      try {
        const alerts = await budgetApi.listRecentAlerts(20);
        if (!mountedRef.current || alerts.length === 0) return;

        const latest = alerts[0].TriggeredAt;

        if (lastSeenRef.current === null) {
          // First fetch — set cursor without showing toasts for old history.
          lastSeenRef.current = latest;
          return;
        }

        const newAlerts = alerts.filter(
          (a) => a.TriggeredAt > lastSeenRef.current!
        );

        for (const alert of newAlerts.reverse()) {
          const isHalt = alert.ActionTaken === "halted";
          addToastRef.current({
            title: `${alert.ProjectName} — ${isHalt ? "Run Halted" : "Budget Alert"}`,
            message: `${alert.RuleName}: cost $${alert.CurrentCost.toFixed(4)} exceeded $${alert.ThresholdUSD.toFixed(4)}`,
            variant: isHalt ? "halt" : "notify",
            href: `/projects/${alert.ProjectID}?tab=budget`,
          });
        }

        if (newAlerts.length > 0) {
          lastSeenRef.current = newAlerts[newAlerts.length - 1].TriggeredAt;
        }
      } catch { /* network errors are non-fatal */ }
    };

    poll();
    const id = setInterval(poll, POLL_INTERVAL_MS);
    return () => { mountedRef.current = false; clearInterval(id); };
  }, []); // no deps — runs once for the app lifetime
}
