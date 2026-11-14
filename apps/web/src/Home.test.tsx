import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Home } from "./Home";
import { useLocale } from "./state/locale";

function renderHome() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <Home />
    </QueryClientProvider>,
  );
}

describe("Home", () => {
  beforeEach(() => useLocale.setState({ locale: "en" }));
  afterEach(() => vi.unstubAllGlobals());

  it("shows loading, then the enabled modules", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ name: "samnak-chat", version: "x", modules: ["auth", "chat"] }), {
          status: 200,
        }),
      ),
    );
    renderHome();
    expect(screen.getByRole("heading", { name: "Samnak Chat" })).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("Loading…");
    expect(await screen.findByText("auth")).toBeInTheDocument();
    expect(screen.getByText("chat")).toBeInTheDocument();
  });

  it("shows an error state", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: { code: "internal", message: "boom" } }), { status: 500 }),
      ),
    );
    renderHome();
    expect(await screen.findByRole("alert")).toHaveTextContent("Could not load server info");
  });

  it("shows an empty state when no modules are enabled", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response(JSON.stringify({ name: "n", version: "v", modules: [] }), { status: 200 })),
    );
    renderHome();
    expect(await screen.findByText("No modules enabled yet")).toBeInTheDocument();
  });
});
