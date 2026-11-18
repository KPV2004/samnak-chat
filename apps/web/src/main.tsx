import { ApiError } from "@samnak/core";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router";
import { App } from "./App";
import { session } from "./session";
import { SessionProvider } from "./state/session";
import { initTheme } from "./state/theme";
import "./index.css";

initTheme();

/**
 * Retries a failed query the default three times, except for a 4xx answer: the server has
 * said no (404, 403), and waiting through retries only delays the error page.
 */
export function retryUnlessClientError(failureCount: number, error: unknown): boolean {
  if (error instanceof ApiError && error.status >= 400 && error.status < 500) return false;
  return failureCount < 3;
}

export const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 15_000, refetchOnWindowFocus: false, retry: retryUnlessClientError } },
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <SessionProvider session={session}>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <App />
        </BrowserRouter>
      </QueryClientProvider>
    </SessionProvider>
  </StrictMode>,
);
