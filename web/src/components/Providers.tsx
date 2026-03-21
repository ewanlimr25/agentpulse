"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { ToastProvider } from "./toast/ToastContext";
import { ToastContainer } from "./toast/ToastContainer";
import { useGlobalAlertPoller } from "@/hooks/useGlobalAlertPoller";

function GlobalAlertPoller() {
  useGlobalAlertPoller();
  return null;
}

export function Providers({ children }: { children: React.ReactNode }) {
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: { staleTime: 15_000, retry: 1 },
        },
      })
  );

  return (
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <GlobalAlertPoller />
        {children}
        <ToastContainer />
      </ToastProvider>
    </QueryClientProvider>
  );
}
