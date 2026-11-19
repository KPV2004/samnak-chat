import type { AuditEntry } from "@samnak/core";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { acme, errorRes, mali, renderApp, somchai, type Routes } from "../test-utils";

const roleAndStatus: AuditEntry = {
  id: "a1",
  actor_id: "u1",
  action: "member.updated",
  target_type: "member",
  target_id: "u2",
  data: { role: { from: "member", to: "admin" }, active: { from: true, to: false } },
  created_at: "2026-01-01T09:00:00Z",
};

const inviteCreated: AuditEntry = {
  id: "a2",
  actor_id: "u1",
  action: "invite.created",
  target_type: "invite",
  target_id: "i1",
  data: {},
  created_at: "2026-01-01T08:00:00Z",
};

const removedLongAgo: AuditEntry = {
  id: "a3",
  actor_id: "u1",
  action: "member.removed",
  target_type: "member",
  target_id: "u9",
  data: {},
  created_at: "2025-12-31T08:00:00Z",
};

function routes(role: "owner" | "member" = "owner", extra: Routes = {}): Routes {
  return {
    "GET /api/v1/workspace": () => ({ items: [{ ...acme, role }] }),
    "GET /api/v1/workspace/members": () => ({ items: [mali, somchai], next_cursor: "" }),
    "GET /api/v1/workspace/audit": (_body, url) => {
      if (url.includes("cursor=c2")) return { items: [removedLongAgo], next_cursor: "" };
      return { items: [roleAndStatus, inviteCreated], next_cursor: "c2" };
    },
    ...extra,
  };
}

function row(text: string) {
  return screen.getByRole("row", { name: new RegExp(text) });
}

describe("Audit log", () => {
  it("names who did what, with the change, and the time in the user's time zone", async () => {
    renderApp({ route: "/audit", routes: routes() });
    // The page is lazy-loaded, so its first render can take longer than the default wait on a busy machine.
    const table = await screen.findByRole("table", { name: "Audit log" }, { timeout: 3000 });

    const change = within(table).getByRole("row", { name: /Changed a member/ });
    expect(within(change).getByText("Mali")).toBeInTheDocument();
    expect(within(change).getByText("Somchai")).toBeInTheDocument();
    expect(change).toHaveTextContent("Role: Member → Admin");
    expect(change).toHaveTextContent("Status: Active → Deactivated");
    // 09:00 UTC is 16:00 in Asia/Bangkok, the time zone of the signed-in user.
    expect(change).toHaveTextContent(/4:00\s?PM/);
    expect(change).not.toHaveTextContent(/9:00\s?AM/);
  });

  it("names the target of an invite by its kind", async () => {
    renderApp({ route: "/audit", routes: routes() });
    expect(await screen.findByRole("row", { name: /Created an invite/ })).toHaveTextContent("Invite");
  });

  it("says a removed member is a former member", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/audit", routes: routes() });
    await screen.findByRole("table", { name: "Audit log" });
    await user.click(screen.getByRole("button", { name: "Load more" }));
    expect(await screen.findByRole("row", { name: /Removed a member/ })).toHaveTextContent("A former member");
  });

  it("loads the next page with its cursor and adds it below", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/audit", routes: routes() });
    await screen.findByRole("row", { name: /Created an invite/ });
    expect(app.called("GET /api/v1/workspace/audit")[0]!.headers["X-Workspace-ID"]).toBe("w1");

    await user.click(screen.getByRole("button", { name: "Load more" }));

    expect(await screen.findByRole("row", { name: /Removed a member/ })).toBeInTheDocument();
    expect(row("Created an invite")).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByRole("button", { name: "Load more" })).not.toBeInTheDocument());
    const urls = app.fetch.mock.calls.map((c) => c[0] as string).filter((u) => u.includes("/workspace/audit"));
    expect(urls.at(-1)).toContain("cursor=c2");
  });

  it("tells a member that only owners and admins can see it, and asks for nothing", async () => {
    const app = renderApp({ route: "/audit", routes: routes("member") });
    expect(await screen.findByText("Only owners and admins can see the audit log.")).toBeInTheDocument();
    expect(app.called("GET /api/v1/workspace/audit")).toHaveLength(0);
  });

  it("is linked from the sidebar for an owner or admin", async () => {
    renderApp({ route: "/", routes: routes() });
    expect(await screen.findByRole("link", { name: "Audit log" })).toHaveAttribute("href", "/audit");
  });

  it("is not linked from the sidebar for a member", async () => {
    renderApp({ route: "/", routes: routes("member") });
    await screen.findByRole("link", { name: "Members" });
    expect(screen.queryByRole("link", { name: "Audit log" })).not.toBeInTheDocument();
  });

  it("says when the log could not be loaded, and retries", async () => {
    const user = userEvent.setup();
    let fail = true;
    renderApp({
      route: "/audit",
      routes: routes("owner", {
        "GET /api/v1/workspace/audit": () => (fail ? errorRes(500, "internal") : { items: [], next_cursor: "" }),
      }),
    });
    expect(await screen.findByRole("alert")).toHaveTextContent("Something went wrong. Try again.");
    fail = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByText("No changes recorded yet.")).toBeInTheDocument();
  });
});
