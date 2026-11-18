import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { acme, chatRoutes, renderApp, type Routes } from "../test-utils";

function routes(modules: string[]): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules }),
    "GET /api/v1/workspace": () => ({ items: [acme] }),
    "GET /api/v1/helpdesk/tickets": () => ({ items: [], next_cursor: "" }),
    "GET /api/v1/helpdesk/queues": () => ({ items: [] }),
  };
}

describe("helpdesk module gating", () => {
  it("shows the Helpdesk link when /meta lists helpdesk", async () => {
    renderApp({ route: "/", routes: routes(["helpdesk"]) });
    const nav = await screen.findByRole("navigation", { name: "Main" });
    expect(await within(nav).findByRole("link", { name: "Helpdesk" })).toHaveAttribute("href", "/helpdesk");
  });

  it("hides the Helpdesk link when /meta does not list helpdesk", async () => {
    renderApp({ route: "/", routes: routes(["chat"]) });
    const nav = await screen.findByRole("navigation", { name: "Main" });
    expect(await within(nav).findByRole("link", { name: "Chat" })).toBeInTheDocument();
    expect(within(nav).queryByRole("link", { name: "Helpdesk" })).not.toBeInTheDocument();
  });

  it("leads to the home page when /helpdesk is opened without the module", async () => {
    renderApp({ route: "/helpdesk", routes: routes([]) });
    expect(await screen.findByText("No modules enabled yet")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Helpdesk" })).not.toBeInTheDocument();
  });
});
