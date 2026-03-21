"use client";

import { useRouter } from "next/navigation";
import { useToast, type Toast } from "./ToastContext";

const borderAccent: Record<Toast["variant"], string> = {
  notify: "border-l-indigo-500",
  info: "border-l-indigo-500",
  halt: "border-l-red-500",
};

export function ToastContainer() {
  const { toasts, removeToast } = useToast();
  const router = useRouter();

  if (toasts.length === 0) return null;

  return (
    <div className="fixed top-4 right-4 z-50 flex flex-col gap-3 w-80">
      {toasts.map((toast) => (
        <div
          key={toast.id}
          className={`rounded-xl border border-l-4 bg-[var(--surface)] shadow-lg p-4 relative ${borderAccent[toast.variant]} ${toast.href ? "cursor-pointer" : ""} ${toast.isExiting ? "toast-exit" : "toast-enter"}`}
          onClick={
            toast.href
              ? () => {
                  router.push(toast.href!);
                  removeToast(toast.id);
                }
              : undefined
          }
        >
          <p className="text-sm font-semibold text-[var(--text)] pr-6">
            {toast.title}
          </p>
          <p className="text-xs text-[var(--text-muted)] mt-1">
            {toast.message}
          </p>
          <button
            type="button"
            aria-label="Dismiss notification"
            onClick={(e) => {
              e.stopPropagation();
              removeToast(toast.id);
            }}
            className="absolute top-3 right-3 text-[var(--text-muted)] hover:text-[var(--text)] text-lg leading-none"
          >
            ×
          </button>
        </div>
      ))}
    </div>
  );
}
