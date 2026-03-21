import { useCallback, useEffect, useRef, useState } from "react";

import { useToast } from "@/components/toast/ToastContext";
import type { BudgetAlert } from "@/lib/types";

const BACKOFF_INITIAL_MS = 1_000;
const BACKOFF_MAX_MS = 30_000;

function buildWsUrl(projectId: string): string {
  const path = `/api/v1/ws/alerts?project_id=${encodeURIComponent(projectId)}`;
  const apiUrl = process.env.NEXT_PUBLIC_API_URL;

  if (apiUrl) {
    const wsBase = apiUrl
      .replace(/^https:\/\//, "wss://")
      .replace(/^http:\/\//, "ws://");
    return `${wsBase.replace(/\/$/, "")}${path}`;
  }

  return `ws://localhost:8080${path}`;
}

export function useAlertWebSocket(projectId: string): { isConnected: boolean } {
  const [isConnected, setIsConnected] = useState(false);
  const { addToast } = useToast();

  // Stable ref so the reconnect closure always calls the current addToast
  const addToastRef = useRef(addToast);
  useEffect(() => {
    addToastRef.current = addToast;
  }, [addToast]);

  const socketRef = useRef<WebSocket | null>(null);
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const mountedRef = useRef(true);
  const attemptRef = useRef(0);

  const connect = useCallback(() => {
    if (!mountedRef.current || !projectId) return;

    const ws = new WebSocket(buildWsUrl(projectId));
    socketRef.current = ws;

    ws.onopen = () => {
      if (!mountedRef.current) {
        ws.close();
        return;
      }
      attemptRef.current = 0;
      setIsConnected(true);
    };

    ws.onmessage = (event: MessageEvent) => {
      if (!mountedRef.current) return;

      let alert: BudgetAlert;
      try {
        alert = JSON.parse(event.data as string) as BudgetAlert;
      } catch {
        return;
      }

      const isHalt = alert.ActionTaken === "halt";
      const runSuffix = alert.RunID ? ` on run ${alert.RunID.slice(0, 8)}` : "";

      addToastRef.current({
        title: `Budget Alert — ${isHalt ? "Run Halted" : "Notified"}`,
        message: `Cost $${alert.CurrentCost.toFixed(4)} exceeded $${alert.ThresholdUSD.toFixed(2)} threshold${runSuffix}`,
        variant: isHalt ? "halt" : "notify",
      });
    };

    const scheduleReconnect = () => {
      if (!mountedRef.current) return;
      setIsConnected(false);

      const delay = Math.min(
        BACKOFF_INITIAL_MS * 2 ** attemptRef.current,
        BACKOFF_MAX_MS,
      );
      attemptRef.current += 1;

      retryTimerRef.current = setTimeout(() => {
        if (mountedRef.current) connect();
      }, delay);
    };

    ws.onclose = scheduleReconnect;
    ws.onerror = scheduleReconnect;
  }, [projectId]); // addToast accessed via ref — intentionally excluded

  useEffect(() => {
    mountedRef.current = true;

    if (projectId) {
      connect();
    }

    return () => {
      mountedRef.current = false;

      if (retryTimerRef.current !== null) {
        clearTimeout(retryTimerRef.current);
        retryTimerRef.current = null;
      }

      if (socketRef.current !== null) {
        // Remove handlers before closing so onclose does not schedule a reconnect
        socketRef.current.onclose = null;
        socketRef.current.onerror = null;
        socketRef.current.close();
        socketRef.current = null;
      }
    };
  }, [connect]);

  return { isConnected };
}
