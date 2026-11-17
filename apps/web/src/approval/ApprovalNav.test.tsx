import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { acme, chatRoutes, renderApp, type Routes } from "../test-utils";

function routes(modules: string[]): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules }),
    "GET /api/v1/workspace": () => ({ items: [acme] }),
    "GET /api/v1/approval/requests": () => ({ items: [], next_cursor: "" }),
  };
}

describe("approval module gating", () => {
  it("shows the Approvals and Groups links when /meta lists approval", async () => {
    renderApp({ route: "/", routes: routes(["approval"]) });
    const nav = await screen.findByRole("navigation", { name: "Main" });
    expect(await within(nav).findByRole("link", { name: "Approvals" })).toHaveAttribute("href", "/approvals");
    expect(within(nav).getByRole("link", { name: "Groups" })).toHaveAttribute("href", "/groups");
  });

  it("hides both links when /meta does not list approval", async () => {
    renderApp({ route: "/", routes: routes(["chat"]) });
    const nav = await screen.findByRole("navigation", { name: "Main" });
    expect(await within(nav).findByRole("link", { name: "Chat" })).toBeInTheDocument();
    expect(within(nav).queryByRole("link", { name: "Approvals" })).not.toBeInTheDocument();
    expect(within(nav).queryByRole("link", { name: "Groups" })).not.toBeInTheDocument();
  });

  it("leads to the home page when /approvals is opened without the module", async () => {
    renderApp({ route: "/approvals", routes: routes([]) });
    expect(await screen.findByText("No modules enabled yet")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Approvals" })).not.toBeInTheDocument();
  });

  it("leads to the home page when /groups is opened without the module", async () => {
    renderApp({ route: "/groups", routes: routes([]) });
    expect(await screen.findByText("No modules enabled yet")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Groups" })).not.toBeInTheDocument();
  });
});
