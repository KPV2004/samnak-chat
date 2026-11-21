import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { acme, chatRoutes, renderApp, type Routes } from "../test-utils";

const queue = (id: string, name: string, agents: string[]) => ({
  id,
  name,
  prefix: name.slice(0, 2).toUpperCase(),
  channel_id: null,
  agent_ids: agents,
  categories: [{ id: `${id}c`, name: "General", is_default: true, queue_id: id }],
});

function routes(modules: string[], role = "member", queues = [queue("q1", "IT", ["u1"]), queue("q2", "Facilities", ["u2"])]): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules }),
    "GET /api/v1/workspace": () => ({ items: [{ ...acme, role }] }),
    "GET /api/v1/helpdesk/tickets": () => ({ items: [], next_cursor: "" }),
    "GET /api/v1/helpdesk/queues": () => ({ items: queues }),
  };
}

describe("helpdesk sidebar", () => {
  it("gives an agent My requests, All queues and one entry per queue they are an agent of", async () => {
    renderApp({ route: "/helpdesk", routes: routes(["helpdesk"]) });
    const nav = await screen.findByRole("navigation", { name: "Helpdesk views" });
    const mine = within(nav).getByRole("link", { name: "My requests" });
    expect(mine).toHaveAttribute("aria-current", "page");
    expect(await within(nav).findByRole("link", { name: "All queues" })).toHaveAttribute("href", "/helpdesk?view=queue");
    expect(within(nav).getByRole("link", { name: "IT" })).toHaveAttribute("href", "/helpdesk?view=queue&queue=q1");
    expect(within(nav).queryByRole("link", { name: "Facilities" })).not.toBeInTheDocument();
    expect(within(nav).queryByRole("link", { name: "Helpdesk settings" })).not.toBeInTheDocument();
    expect(screen.getAllByRole("link", { name: "New ticket" })[0]).toHaveAttribute("href", "/helpdesk?new=1");
  });

  it("gives a member who is no agent only My requests", async () => {
    renderApp({ route: "/helpdesk", routes: routes(["helpdesk"], "member", [queue("q2", "Facilities", ["u2"])]) });
    const nav = await screen.findByRole("navigation", { name: "Helpdesk views" });
    await within(nav).findByRole("link", { name: "My requests" });
    expect(within(nav).queryByRole("link", { name: "All queues" })).not.toBeInTheDocument();
  });

  it("gives an admin the queue list and Settings", async () => {
    renderApp({ route: "/helpdesk", routes: routes(["helpdesk"], "admin", [queue("q2", "Facilities", ["u2"])]) });
    const nav = await screen.findByRole("navigation", { name: "Helpdesk views" });
    expect(await within(nav).findByRole("link", { name: "All queues" })).toBeInTheDocument();
    expect(within(nav).getByRole("link", { name: "Helpdesk settings" })).toHaveAttribute("href", "/helpdesk/settings");
  });

  it("drives the list and the queue filter from the URL, and marks the view", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/helpdesk", routes: routes(["helpdesk"]) });
    const nav = await screen.findByRole("navigation", { name: "Helpdesk views" });
    await user.click(await within(nav).findByRole("link", { name: "IT" }));
    expect(within(nav).getByRole("link", { name: "IT" })).toHaveAttribute("aria-current", "page");
    expect(within(nav).getByRole("link", { name: "My requests" })).not.toHaveAttribute("aria-current");
    // The page's own queue filter follows the URL.
    expect(await screen.findByRole("combobox", { name: "Queue" })).toHaveValue("q1");
    await user.click(within(nav).getByRole("link", { name: "All queues" }));
    expect(screen.getByRole("combobox", { name: "Queue" })).toHaveValue("");
    expect(within(nav).getByRole("link", { name: "All queues" })).toHaveAttribute("aria-current", "page");
  });

  it("opens the New ticket dialog from the sidebar button", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/helpdesk", routes: routes(["helpdesk"]) });
    await user.click((await screen.findAllByRole("link", { name: "New ticket" }))[0]!);
    expect(await screen.findByRole("dialog", { name: "New ticket" })).toBeInTheDocument();
  });

  it("is not an app on the rail, and leads to Messenger, when /meta does not list helpdesk", async () => {
    renderApp({ route: "/helpdesk", routes: routes(["chat"]) });
    expect(await screen.findByRole("searchbox", { name: "Search conversations" }, { timeout: 5000 })).toBeInTheDocument();
    expect(within(screen.getByRole("navigation", { name: "Apps" })).queryByRole("link", { name: "Helpdesk" })).not.toBeInTheDocument();
  });
});
