"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { emailDigestApi } from "@/lib/api";
import { usePushNotifications } from "@/hooks/usePushNotifications";

interface Props {
  projectId: string;
}

export function NotificationPreferences({ projectId }: Props) {
  const qc = useQueryClient();
  const { state: pushState, enable: enablePush, disable: disablePush } = usePushNotifications(projectId);

  const { data: digestConfig } = useQuery({
    queryKey: ["emailDigest", projectId],
    queryFn: () => emailDigestApi.get(projectId),
  });

  const [email, setEmail] = useState(digestConfig?.RecipientEmail ?? "");
  const [schedule, setSchedule] = useState<"daily" | "hourly">(digestConfig?.Schedule ?? "daily");
  const [digestEnabled, setDigestEnabled] = useState(digestConfig?.Enabled ?? false);

  const digestMutation = useMutation({
    mutationFn: (body: { enabled: boolean; recipient_email: string; schedule: "daily" | "hourly" }) =>
      emailDigestApi.upsert(projectId, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["emailDigest", projectId] }),
  });

  function handleDigestSave() {
    digestMutation.mutate({ enabled: digestEnabled, recipient_email: email, schedule });
  }

  return (
    <div className="border border-[var(--border)] rounded-xl px-6 py-5 space-y-5">
      <h3 className="text-sm font-semibold text-[var(--text)]">Notification Preferences</h3>

      {/* Browser push */}
      <div className="flex items-center justify-between">
        <div>
          <p className="text-sm text-[var(--text)]">Browser Push Notifications</p>
          <p className="text-xs text-[var(--text-muted)] mt-0.5">
            {pushState === "unsupported"
              ? "Not supported in this browser"
              : pushState === "denied"
              ? "Blocked by browser — allow in browser settings"
              : "Get notified in this browser when alerts fire"}
          </p>
        </div>
        {pushState !== "unsupported" && (
          <button
            onClick={pushState === "granted" ? disablePush : enablePush}
            disabled={pushState === "loading" || pushState === "denied"}
            className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors disabled:opacity-40 ${
              pushState === "granted" ? "bg-indigo-600" : "bg-zinc-600"
            }`}
          >
            <span
              className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white transition-transform ${
                pushState === "granted" ? "translate-x-4" : "translate-x-1"
              }`}
            />
          </button>
        )}
      </div>

      {/* Email digest */}
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm text-[var(--text)]">Email Digest</p>
            <p className="text-xs text-[var(--text-muted)] mt-0.5">
              Periodic summary of alert events
            </p>
          </div>
          <button
            type="button"
            onClick={() => setDigestEnabled((v) => !v)}
            className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${
              digestEnabled ? "bg-indigo-600" : "bg-zinc-600"
            }`}
          >
            <span
              className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white transition-transform ${
                digestEnabled ? "translate-x-4" : "translate-x-1"
              }`}
            />
          </button>
        </div>

        {digestEnabled && (
          <div className="space-y-2 pl-0">
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="you@example.com"
              className="w-full bg-[var(--surface-2)] border border-[var(--border)] rounded-lg px-3 py-2 text-sm text-[var(--text)] focus:outline-none focus:border-indigo-500"
            />
            <select
              value={schedule}
              onChange={(e) => setSchedule(e.target.value as "daily" | "hourly")}
              className="w-full bg-[var(--surface-2)] border border-[var(--border)] rounded-lg px-3 py-2 text-sm text-[var(--text)] focus:outline-none focus:border-indigo-500"
            >
              <option value="daily">Daily digest</option>
              <option value="hourly">Hourly digest</option>
            </select>
          </div>
        )}

        {digestConfig?.LastError && (
          <p className="text-xs text-red-400 mt-1">Last send error: {digestConfig.LastError}</p>
        )}

        <button
          onClick={handleDigestSave}
          disabled={digestMutation.isPending}
          className="text-xs bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white px-3 py-1.5 rounded-lg transition-colors"
        >
          {digestMutation.isPending ? "Saving…" : "Save preferences"}
        </button>
      </div>
    </div>
  );
}
