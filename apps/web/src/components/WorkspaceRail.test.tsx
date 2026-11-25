import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { acme, channel, renderApp } from "../test-utils";
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
    const app = renderApp({
      route: "/",
      routes: { "GET /api/v1/workspace": () => ({ items: [acme, beta] }) },
    });
    fireEvent.click(await screen.findByRole("button", { name: "Workspace Acme" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "Beta Labs" }));
    expect(await screen.findByRole("button", { name: "Workspace Beta Labs" })).toBeInTheDocument();
    expect(app.store.load()?.workspaceId).toBe("w2");
  });

  it("links to workspace creation", async () => {
    renderApp({ route: "/" });
    fireEvent.click(await screen.findByRole("button", { name: "Workspace Acme" }));
    expect(await screen.findByRole("menuitem", { name: "New workspace" })).toHaveAttribute("href", "/workspaces/new");
  });
});

describe("app rail", () => {
  const modules = (list: string[]) => ({ "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules: list }) });

  it("lists the apps whose modules are on, with names, and marks the current one", async () => {
    renderApp({ route: "/helpdesk", routes: modules(["chat", "task", "approval", "helpdesk"]) });
    const apps = await screen.findByRole("navigation", { name: "Apps" });
    const names = async () => within(apps).getAllByRole("link").map((a) => a.textContent);
    await within(apps).findByRole("link", { name: "Helpdesk" });
    expect(await names()).toEqual(["Home", "Messenger", "Tasks", "Approvals", "Helpdesk", "Workspace"]);
    expect(within(apps).getByRole("link", { name: "Helpdesk" })).toHaveAttribute("aria-current", "page");
    expect(within(apps).getByRole("link", { name: "Messenger" })).not.toHaveAttribute("aria-current");
    expect(within(apps).getByRole("link", { name: "Approvals" })).toHaveAttribute("href", "/approvals");
  });

  it("leaves out an app whose module is off", async () => {
    renderApp({ route: "/", routes: modules(["chat"]) });
    const apps = await screen.findByRole("navigation", { name: "Apps" });
    await within(apps).findByRole("link", { name: "Messenger" });
    expect(within(apps).queryByRole("link", { name: "Tasks" })).not.toBeInTheDocument();
    expect(within(apps).queryByRole("link", { name: "Approvals" })).not.toBeInTheDocument();
    expect(within(apps).queryByRole("link", { name: "Helpdesk" })).not.toBeInTheDocument();
    expect(within(apps).getByRole("link", { name: "Workspace" })).toBeInTheDocument();
  });

  it("marks Workspace on the groups page, and goes to Messenger on click", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/groups", routes: modules(["chat", "approval"]) });
    const apps = await screen.findByRole("navigation", { name: "Apps" });
    expect(await within(apps).findByRole("link", { name: "Workspace" })).toHaveAttribute("aria-current", "page");
    expect(within(apps).getByRole("link", { name: "Approvals" })).not.toHaveAttribute("aria-current");
    await user.click(within(apps).getByRole("link", { name: "Messenger" }));
    expect(within(apps).getByRole("link", { name: "Messenger" })).toHaveAttribute("aria-current", "page");
  });

  it("badges Messenger with the unread chats, muted ones left out", async () => {
    renderApp({
      route: "/",
      routes: {
        ...modules(["chat", "notify"]),
        "GET /api/v1/notify/preferences": () => ({ disabled_kinds: [], muted_channel_ids: ["c2"], dnd_start: null, dnd_end: null }),
        "GET /api/v1/chat/channels": () => ({
          items: [channel("c1", { last_seq: 4, unread_count: 4 }), channel("c2", { last_seq: 9, unread_count: 9 })],
        }),
      },
    });
    const apps = await screen.findByRole("navigation", { name: "Apps" });
    await waitFor(() => expect(within(apps).getByRole("link", { name: /Messenger/ })).toHaveTextContent("4 unread"));
  });
});
