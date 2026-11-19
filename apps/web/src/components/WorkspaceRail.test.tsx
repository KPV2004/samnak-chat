import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { acme, renderApp } from "../test-utils";
import { workspaceInitials } from "./WorkspaceRail";

const beta = { id: "w2", name: "Beta Labs", slug: "beta", role: "member" };

describe("workspaceInitials", () => {
  it("takes two words, else two letters", () => {
    expect(workspaceInitials("Beta Labs")).toBe("BL");
    expect(workspaceInitials("acme")).toBe("AC");
    expect(workspaceInitials("  ")).toBe("?");
  });
});

describe("workspace rail", () => {
  it("has a tile per workspace, marks the current one and switches on click", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/",
      routes: { "GET /api/v1/workspace": () => ({ items: [acme, beta] }) },
    });
    const rail = await screen.findByRole("navigation", { name: "Workspaces" });
    expect(await within(rail).findByRole("button", { name: "Acme" })).toHaveAttribute("aria-current", "true");
    expect(within(rail).getByRole("button", { name: "Beta Labs" })).not.toHaveAttribute("aria-current");

    await user.click(within(rail).getByRole("button", { name: "Beta Labs" }));
    expect(within(rail).getByRole("button", { name: "Beta Labs" })).toHaveAttribute("aria-current", "true");
    expect(app.store.load()?.workspaceId).toBe("w2");
  });

  it("links to workspace creation", async () => {
    renderApp({ route: "/" });
    const rail = await screen.findByRole("navigation", { name: "Workspaces" });
    expect(await within(rail).findByRole("link", { name: "New workspace" })).toHaveAttribute("href", "/workspaces/new");
  });
});
