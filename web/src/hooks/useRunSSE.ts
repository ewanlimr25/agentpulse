"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import type { Span, Run } from "@/lib/types";
import { getApiKey } from "@/lib/api-keys";

const BASE_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
const POLL_PATH = (runId: string) => `${BASE_URL}/api/v1/runs/${runId}/live`;

interface SSEState {
  spans: Span[];
  run: Run | null;
  isLive: boolean;
  isConnected: boolean;
  fetchedAt: number | null; // server-side fetch timestamp (ms)
}

export function useRunSSE(
  runId: string,
  projectId: string,
  enabled: boolean
): SSEState {
  const [state, setState] = useState<SSEState>({
    spans: [],
    run: null,
    isLive: false,
    isConnected: false,
    fetchedAt: null,
  });

  const abortRef = useRef<AbortController | null>(null);
  const retryRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const retryDelayRef = useRef(1000);

  const scheduleRetry = useCallback((connectFn: () => void) => {
    const delay = Math.min(retryDelayRef.current, 10000);
    retryDelayRef.current = Math.min(delay * 2, 10000);
    retryRef.current = setTimeout(connectFn, delay);
  }, []);

  const connect = useCallback(() => {
    if (!enabled) return;

    const apiKey = getApiKey(projectId);
    if (!apiKey) return;

    const ctrl = new AbortController();
    abortRef.current = ctrl;

    const headers: Record<string, string> = {
      Authorization: `Bearer ${apiKey}`,
      Accept: "text/event-stream",
    };

    const url = `${POLL_PATH(runId)}?idle_timeout=120`;

    fetch(url, { headers, signal: ctrl.signal })
      .then(async (res) => {
        if (!res.ok || !res.body) {
          throw new Error(`SSE connect failed: ${res.status}`);
        }

        const connectedAt = Date.now();
        setState((s) => ({ ...s, isConnected: true, isLive: true, fetchedAt: connectedAt }));
        retryDelayRef.current = 1000; // reset backoff on successful connect

        const reader = res.body.getReader();
        const decoder = new TextDecoder();
        let buffer = "";

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          buffer += decoder.decode(value, { stream: true });

          // Split on double-newline (SSE message boundary)
          const messages = buffer.split("\n\n");
          buffer = messages.pop() ?? ""; // keep incomplete last chunk

          for (const msg of messages) {
            if (!msg.trim()) continue;

            let eventType = "message";
            const dataLines: string[] = [];

            for (const line of msg.split("\n")) {
              if (line.startsWith("event:")) {
                eventType = line.slice("event:".length).trim();
              } else if (line.startsWith("data:")) {
                dataLines.push(line.slice("data:".length).trim());
              }
              // skip comment lines (keepalive)
            }

            if (dataLines.length === 0) continue;
            const dataStr = dataLines.join("\n");

            try {
              const payload = JSON.parse(dataStr);

              if (eventType === "initial") {
                // payload is Span[] — replace entire span list
                setState((s) => ({ ...s, spans: Array.isArray(payload) ? payload : [] }));
              } else if (eventType === "span") {
                // payload is a single Span — immutable append
                setState((s) => ({
                  ...s,
                  spans: [...s.spans, payload as Span],
                }));
              } else if (eventType === "metrics") {
                // payload is updated Run aggregate
                setState((s) => ({ ...s, run: payload as Run }));
              } else if (eventType === "done") {
                setState((s) => ({ ...s, isLive: false, isConnected: false }));
                return; // clean exit, no retry
              }
            } catch {
              // malformed JSON — skip silently
            }
          }
        }

        // Stream ended without 'done' event — treat as connection drop, retry
        setState((s) => ({ ...s, isConnected: false }));
        scheduleRetry(connect);
      })
      .catch((err: unknown) => {
        if (err instanceof DOMException && err.name === "AbortError") return; // intentional
        setState((s) => ({ ...s, isConnected: false }));
        scheduleRetry(connect);
      });
  }, [runId, projectId, enabled, scheduleRetry]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!enabled) {
      setState({ spans: [], run: null, isLive: false, isConnected: false, fetchedAt: null });
      return;
    }

    connect();

    return () => {
      abortRef.current?.abort();
      if (retryRef.current) clearTimeout(retryRef.current);
    };
  }, [enabled, connect]);

  return state;
}
