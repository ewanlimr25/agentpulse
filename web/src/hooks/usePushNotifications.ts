"use client";

import { useState, useEffect, useCallback } from "react";
import { pushApi } from "@/lib/api";

type PushState = "unsupported" | "default" | "granted" | "denied" | "loading";

export function usePushNotifications(projectId: string) {
  const [state, setState] = useState<PushState>("loading");

  const isSupported =
    typeof window !== "undefined" &&
    "serviceWorker" in navigator &&
    "PushManager" in window &&
    "showNotification" in ServiceWorkerRegistration.prototype;

  useEffect(() => {
    if (!isSupported) {
      setState("unsupported");
      return;
    }
    setState(Notification.permission as PushState);
  }, [isSupported]);

  const enable = useCallback(async () => {
    if (!isSupported) return;
    setState("loading");
    try {
      const reg = await navigator.serviceWorker.register("/sw.js");
      await navigator.serviceWorker.ready;

      const { key } = await pushApi.getVapidKey(projectId);
      const keyBytes = urlBase64ToUint8Array(key);

      const sub = await reg.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: keyBytes,
      });

      const json = sub.toJSON();
      await pushApi.subscribe(projectId, {
        endpoint: json.endpoint!,
        p256dh_key: json.keys!.p256dh,
        auth_key: json.keys!.auth,
        vapid_public_key: key,
        user_agent: navigator.userAgent,
      });

      setState("granted");
    } catch (err) {
      console.warn("push subscribe failed", err);
      setState(Notification.permission as PushState);
    }
  }, [isSupported, projectId]);

  const disable = useCallback(async () => {
    if (!isSupported) return;
    try {
      const reg = await navigator.serviceWorker.getRegistration("/sw.js");
      if (!reg) return;
      const sub = await reg.pushManager.getSubscription();
      if (sub) {
        await pushApi.unsubscribe(projectId, sub.endpoint);
        await sub.unsubscribe();
      }
      setState("default");
    } catch (err) {
      console.warn("push unsubscribe failed", err);
    }
  }, [isSupported, projectId]);

  return { state, enable, disable };
}

function urlBase64ToUint8Array(base64String: string): Uint8Array<ArrayBuffer> {
  const padding = "=".repeat((4 - (base64String.length % 4)) % 4);
  const base64 = (base64String + padding).replace(/-/g, "+").replace(/_/g, "/");
  const rawData = atob(base64);
  const arr = new Uint8Array(rawData.length);
  for (let i = 0; i < rawData.length; i++) {
    arr[i] = rawData.charCodeAt(i);
  }
  return arr;
}
