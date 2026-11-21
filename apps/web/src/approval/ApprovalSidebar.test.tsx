import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { acme, chatRoutes, renderApp, type Routes } from "../test-utils";

const waiting = {
  id: "r2",
  type: "general",
  status: "pending",
  payload: { subject: "Laptop purchase", details: "", currency: "THB" },
  requester_id: "u2",
  approver_id: "u1",
  message_id: null,
  reason: "",
  decided_at: null,
  created_at: "2026-11-17T09:00:00Z",
  updated_at: "2026-11-17T09:00:00Z",
};

function routes(modules: string[]): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules }),
    "GET /api/v1/workspace": () => ({ items: [acme] }),
    "GET /api/v1/approval/requests": (_b, url) => ({ items: url.includes("role=approver") ? [waiting] : [], next_cursor: "" }),
  };
}

describe("approval sidebar", () => {
  it("shows the two views, the waiting count and a New request button, and nothing of other apps", async () => {
    renderApp({ route: "/approvals", routes: routes(["approval", "task"]) });
    const views = await screen.findByRole("navigation", { name: "Approval views" });
    const waitingLink = await within(views).findByRole("link", { name: /Waiting for me/ });
    expect(waitingLink).toHaveAttribute("href", "/approvals");
    expect(waitingLink).toHaveAttribute("aria-current", "page");
    await waitFor(() => expect(waitingLink).toHaveTextContent("1 waiting for me"));
    const mine = within(views).getByRole("link", { name: "My requests" });
    expect(mine).toHaveAttribute("href", "/approvals?view=mine");
    expect(mine).not.toHaveAttribute("aria-current");
    expect(screen.getAllByRole("link", { name: "New request" })[0]).toHaveAttribute("href", "/approvals?new=1");
    // Only this app: no conversation list, no task views, no members.
    expect(screen.queryByRole("navigation", { name: "Task views" })).not.toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Channels and messages" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Members" })).not.toBeInTheDocument();
  });

  it("switches the list from the sidebar and marks the view", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/approvals", routes: routes(["approval"]) });
    await user.click(await screen.findByRole("link", { name: "My requests" }));
    expect(await screen.findByRole("list", { name: "My requests" }).catch(() => null)).toBeNull();
    expect(await screen.findByText("You have not sent any requests yet.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "My requests" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("link", { name: /Waiting for me/ })).not.toHaveAttribute("aria-current");
  });

  it("opens the New request dialog from the sidebar button", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/approvals", routes: routes(["approval"]) });
    await user.click((await screen.findAllByRole("link", { name: "New request" }))[0]!);
    expect(await screen.findByRole("dialog", { name: "New request" })).toBeInTheDocument();
  });

  it("leaves the Approvals app off the rail, and the page to Messenger, when /meta does not list approval", async () => {
    renderApp({ route: "/approvals", routes: routes(["chat"]) });
    expect(await screen.findByRole("searchbox", { name: "Search conversations" }, { timeout: 5000 })).toBeInTheDocument();
    const apps = screen.getByRole("navigation", { name: "Apps" });
    expect(within(apps).queryByRole("link", { name: "Approvals" })).not.toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Approval views" })).not.toBeInTheDocument();
  });

  it("leads to Messenger when /groups is opened without the module", async () => {
    renderApp({ route: "/groups", routes: routes([]) });
    expect(await screen.findByRole("searchbox", { name: "Search conversations" }, { timeout: 5000 })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Groups" })).not.toBeInTheDocument();
  });
});
