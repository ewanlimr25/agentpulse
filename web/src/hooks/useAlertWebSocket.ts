import { useEffect, useRef } from "react";

import { budgetApi, alertsApi } from "@/lib/api";
import { useToast } from "@/components/toast/ToastContext";
import { SIGNAL_LABELS, formatSignalValue, COMPARE_LABELS } from "@/components/alerts/alertUtils";
import type { SignalType, CompareOp } from "@/lib/types";

const POLL_INTERVAL_MS = 5_000;

/**
 * Polls budget alerts every 5 s for a specific project and fires toasts for
 * new ones. Used by BudgetSection to keep the per-project alert table fresh.
 */
export function useAlertWebSocket(projectId: string): { isConnected: boolean } {
  const { addToast } = useToast();
  const addToastRef = useRef(addToast);
  useEffect(() => { addToastRef.current = addToast; }, [addToast]);

  const lastSeenRef = useRef<string | null>(null);
  const lastSeenSignalRef = useRef<string | null>(null);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    if (!projectId) return;

    const poll = async () => {
      if (!mountedRef.current) return;
      try {
        const alerts = await budgetApi.listAlerts(projectId, 10);
        if (!mountedRef.current || alerts.length === 0) return;

        const latest = alerts[0].TriggeredAt;
        if (lastSeenRef.current === null) {
          lastSeenRef.current = latest;
          return;
        }

        const newAlerts = alerts.filter(
          (a) => a.TriggeredAt > lastSeenRef.current!
        );
        for (const alert of newAlerts.reverse()) {
          const isHalt = alert.ActionTaken === "halted";
          const runSuffix = alert.RunID ? ` on run ${alert.RunID.slice(0, 8)}` : "";
          addToastRef.current({
            title: `Budget Alert — ${isHalt ? "Run Halted" : "Notified"}`,
            message: `Cost $${alert.CurrentCost.toFixed(4)} exceeded $${alert.ThresholdUSD.toFixed(4)} threshold${runSuffix}`,
            variant: isHalt ? "halt" : "notify",
          });
        }
        if (newAlerts.length > 0) {
          lastSeenRef.current = newAlerts[newAlerts.length - 1].TriggeredAt;
        }
      } catch { /* swallow — table UI shows errors */ }
    };

    const pollSignal = async () => {
      if (!mountedRef.current) return;
      try {
        const events = await alertsApi.listEvents(projectId, 10);
        if (!mountedRef.current || events.length === 0) return;

        const latest = events[0].TriggeredAt;
        if (lastSeenSignalRef.current === null) {
          lastSeenSignalRef.current = latest;
          return;
        }

        const newEvents = events.filter((e) => e.TriggeredAt > lastSeenSignalRef.current!);
        for (const evt of newEvents.reverse()) {
          const label = SIGNAL_LABELS[evt.SignalType as SignalType] ?? evt.SignalType;
          const val = formatSignalValue(evt.SignalType as SignalType, evt.CurrentValue);
          const thr = formatSignalValue(evt.SignalType as SignalType, evt.Threshold);
          const dir = COMPARE_LABELS[evt.CompareOp as CompareOp] ?? evt.CompareOp;
          addToastRef.current({
            title: `Alert — ${label}`,
            message: `${val} is ${dir} threshold of ${thr}`,
            variant: "notify",
          });
        }
        if (newEvents.length > 0) {
          lastSeenSignalRef.current = newEvents[newEvents.length - 1].TriggeredAt;
        }
      } catch { /* swallow */ }
    };

    poll();
    pollSignal();
    const id = setInterval(poll, POLL_INTERVAL_MS);
    const id2 = setInterval(pollSignal, POLL_INTERVAL_MS);
    return () => { mountedRef.current = false; clearInterval(id); clearInterval(id2); };
  }, [projectId]);

  return { isConnected: true };
}
