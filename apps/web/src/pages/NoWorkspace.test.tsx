import { screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderApp, type Routes } from "../test-utils";

const routes: Routes = {
  "GET /api/v1/workspace": () => ({ items: [] }),
  "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules: ["notify"] }),
};

afterEach(() => vi.unstubAllGlobals());

describe("a user without a workspace", () => {
  it("sees one honest next step on the chat page and no workspace-only requests (BT-7)", async () => {
    const app = renderApp({ route: "/chat", routes, signedIn: { workspaceId: null } });

    expect(await screen.findByText("You are not in any workspace yet.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "New workspace" })).toHaveAttribute("href", "/workspaces/new");
    expect(screen.queryByText("Loading")).not.toBeInTheDocument();
    const asked = app.fetch.mock.calls.map((call) => call[0] as string);
    expect(asked.some((url) => url.includes("/notify/"))).toBe(false);
    expect(asked.some((url) => url.includes("/chat/channels"))).toBe(false);
  });
});
