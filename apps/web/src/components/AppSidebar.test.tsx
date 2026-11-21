import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import { useLocale } from "../state/locale";
import { acme, channel, chatRoutes, renderApp, type Routes } from "../test-utils";

const all = ["chat", "task", "approval", "helpdesk", "notify"];

function routes(modules = all, role = "owner"): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules }),
    "GET /api/v1/workspace": () => ({ items: [{ ...acme, role }] }),
    "GET /api/v1/chat/channels": () => ({ items: [channel("c1", { name: "general" })] }),
    "GET /api/v1/task/items": () => ({ items: [], next_cursor: "" }),
    "GET /api/v1/approval/requests": () => ({ items: [], next_cursor: "" }),
    "GET /api/v1/helpdesk/tickets": () => ({ items: [], next_cursor: "" }),
    "GET /api/v1/helpdesk/queues": () => ({ items: [] }),
  };
}

beforeEach(() => {
  localStorage.clear();
  useLocale.setState({ locale: "en" });
});

describe("one sidebar per app", () => {
  it("opens Messenger from the home page, with only the conversation list in the sidebar", async () => {
    renderApp({ route: "/", routes: routes() });
    expect(await screen.findByRole("navigation", { name: "Channels and messages" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Members" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Profile" })).not.toBeInTheDocument();
    expect(screen.queryByText("Apps", { selector: "button *" })).not.toBeInTheDocument();
  });

  it("switches the sidebar with the rail: each app shows only its own", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/chat", routes: routes() });
    const apps = await screen.findByRole("navigation", { name: "Apps" });
    for (const [app, sidebar, others] of [
      ["Tasks", "Task views", ["Approval views", "Helpdesk views", "Workspace"]],
      ["Approvals", "Approval views", ["Task views", "Helpdesk views", "Workspace"]],
      ["Helpdesk", "Helpdesk views", ["Task views", "Approval views", "Workspace"]],
    ] as const) {
      await user.click(await within(apps).findByRole("link", { name: app }));
      expect(await screen.findByRole("navigation", { name: sidebar })).toBeInTheDocument();
      expect(within(apps).getByRole("link", { name: app })).toHaveAttribute("aria-current", "page");
      for (const other of others) {
        // "Workspace" is also the rail's own link, so look for the sidebar by its Members link.
        if (other === "Workspace") expect(screen.queryByRole("link", { name: "Members" })).not.toBeInTheDocument();
        else expect(screen.queryByRole("navigation", { name: other })).not.toBeInTheDocument();
      }
      expect(screen.queryByRole("navigation", { name: "Channels and messages" })).not.toBeInTheDocument();
    }
    await user.click(within(apps).getByRole("link", { name: /Messenger/ }));
    expect(await screen.findByRole("navigation", { name: "Channels and messages" })).toBeInTheDocument();
  });

  it("names the app in the sidebar header, beside the workspace", async () => {
    renderApp({ route: "/tasks", routes: routes() });
    const sidebar = (await screen.findByRole("navigation", { name: "Task views" })).closest("div.on-rail")!;
    expect(within(sidebar as HTMLElement).getByText("Tasks")).toBeInTheDocument();
    expect(await within(sidebar as HTMLElement).findByText("Acme")).toBeInTheDocument();
  });
});

describe("workspace sidebar", () => {
  it("links Members, Invites, Groups and Audit log for an owner, and marks the page", async () => {
    renderApp({ route: "/groups", routes: routes() });
    const nav = await screen.findByRole("navigation", { name: "Workspace" });
    expect(await within(nav).findByRole("link", { name: "Groups" })).toHaveAttribute("aria-current", "page");
    expect(within(nav).getByRole("link", { name: "Members" })).toHaveAttribute("href", "/members");
    expect(within(nav).getByRole("link", { name: "Invites" })).toHaveAttribute("href", "/invites");
    expect(within(nav).getByRole("link", { name: "Audit log" })).toHaveAttribute("href", "/audit");
  });

  it("hides the Audit log from a member, and Groups while approval is off", async () => {
    renderApp({ route: "/members", routes: routes(["chat"], "member") });
    const nav = await screen.findByRole("navigation", { name: "Workspace" });
    await within(nav).findByRole("link", { name: "Members" });
    expect(within(nav).queryByRole("link", { name: "Audit log" })).not.toBeInTheDocument();
    expect(within(nav).queryByRole("link", { name: "Groups" })).not.toBeInTheDocument();
  });
});

describe("account", () => {
  it("shows a small Account sidebar on the account pages, and no app is current on the rail", async () => {
    renderApp({ route: "/devices", routes: routes() });
    const nav = await screen.findByRole("navigation", { name: "Account" });
    expect(within(nav).getByRole("link", { name: "Devices" })).toHaveAttribute("aria-current", "page");
    expect(within(nav).getByRole("link", { name: "Profile" })).toHaveAttribute("href", "/profile");
    expect(await within(nav).findByRole("link", { name: "Notification settings" })).toHaveAttribute("href", "/notifications/settings");
    const apps = screen.getByRole("navigation", { name: "Apps" });
    for (const link of within(apps).getAllByRole("link")) expect(link).not.toHaveAttribute("aria-current");
  });

  it("opens the account menu from the rail with the account links, theme, language and Sign out", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/chat", routes: routes() });
    await user.click(await screen.findByRole("button", { name: "Account menu" }));
    const menu = await screen.findByRole("menu");
    expect(within(menu).getByText("me@example.com")).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Profile" })).toHaveAttribute("href", "/profile");
    expect(within(menu).getByRole("menuitem", { name: "Devices" })).toHaveAttribute("href", "/devices");
    expect(within(menu).getByRole("menuitem", { name: "Notification settings" })).toHaveAttribute("href", "/notifications/settings");
    expect(within(menu).getByRole("menuitem", { name: "Theme" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Language" })).toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "Sign out" })).toBeInTheDocument();
  });

  it("leaves Notification settings out of the menu while notify is off", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/chat", routes: routes(["chat"]) });
    await user.click(await screen.findByRole("button", { name: "Account menu" }));
    const menu = await screen.findByRole("menu");
    expect(within(menu).queryByRole("menuitem", { name: "Notification settings" })).not.toBeInTheDocument();
  });

  it("changes the language from the menu", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/chat", routes: routes() });
    await user.click(await screen.findByRole("button", { name: "Account menu" }));
    const trigger = await screen.findByRole("menuitem", { name: "Language" });
    trigger.focus();
    await user.keyboard("{ArrowRight}");
    const thai = await screen.findByRole("menuitemradio", { name: "ไทย" });
    thai.focus();
    await user.keyboard("{Enter}");
    await waitFor(() => expect(useLocale.getState().locale).toBe("th"));
  });
});
