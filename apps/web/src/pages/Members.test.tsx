import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { acme, errorRes, expectAlert, renderApp } from "../test-utils";

const mali = { user_id: "u1", display_name: "Mali", email: "me@example.com", role: "owner", status: "active", joined_at: "2026-01-01T00:00:00Z" };
const somchai = { user_id: "u2", display_name: "Somchai", email: "som@example.com", role: "member", status: "active", joined_at: "2026-02-01T00:00:00Z" };

function members(role = "owner") {
  return {
    routes: {
      "GET /api/v1/workspace": () => ({ items: [{ ...acme, role }] }),
      "GET /api/v1/workspace/members": () => ({ items: [mali, somchai], next_cursor: null }),
    },
  };
}

function row(name: string) {
  return screen.getByRole("row", { name: new RegExp(name) });
}

describe("Members", () => {
  it("lists members and sends the workspace header", async () => {
    const app = renderApp({ route: "/members", ...members() });
    expect(await screen.findByText("Somchai")).toBeInTheDocument();
    expect(screen.getByText("som@example.com")).toBeInTheDocument();
    expect(app.called("GET /api/v1/workspace/members")[0]!.headers["X-Workspace-ID"]).toBe("w1");
  });

  it("changes a role", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/members",
      routes: { ...members().routes, "PATCH /api/v1/workspace/members/u2": (b) => ({ ...somchai, ...b }) },
    });
    await screen.findByText("Somchai");
    await user.selectOptions(screen.getByLabelText("Role of Somchai"), "admin");
    await screen.findByRole("row", { name: /Somchai/ });
    expect(app.called("PATCH /api/v1/workspace/members/u2")[0]!.body).toEqual({ role: "admin" });
  });

  it("deactivates and reactivates", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/members",
      routes: { ...members().routes, "PATCH /api/v1/workspace/members/u2": (b) => ({ ...somchai, ...b }) },
    });
    await screen.findByText("Somchai");
    await user.click(within(row("Somchai")).getByRole("button", { name: "Deactivate Somchai" }));
    expect(app.called("PATCH /api/v1/workspace/members/u2")[0]!.body).toEqual({ status: "deactivated" });
  });

  it("removes only after a confirmation", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/members",
      routes: { ...members().routes, "DELETE /api/v1/workspace/members/u2": () => undefined },
    });
    await screen.findByText("Somchai");
    await user.click(within(row("Somchai")).getByRole("button", { name: "Remove Somchai" }));
    expect(app.called("DELETE /api/v1/workspace/members/u2")).toHaveLength(0);
    await user.click(screen.getByRole("button", { name: "Confirm removing Somchai" }));
    expect(app.called("DELETE /api/v1/workspace/members/u2")).toHaveLength(1);
  });

  it("shows a translated error, e.g. when the last owner would be lost", async () => {
    const user = userEvent.setup();
    renderApp({
      route: "/members",
      routes: { ...members().routes, "PATCH /api/v1/workspace/members/u2": () => errorRes(409, "conflict") },
    });
    await screen.findByText("Somchai");
    await user.selectOptions(screen.getByLabelText("Role of Somchai"), "guest");
    await expectAlert("conflicts");
  });

  it("offers owner only to owners; admins cannot edit owners", async () => {
    renderApp({ route: "/members", ...members("admin") });
    await screen.findByText("Somchai");
    const options = within(screen.getByLabelText("Role of Somchai")).getAllByRole("option");
    expect(options.map((o) => o.textContent)).toEqual(["Admin", "Manager", "Member", "Guest"]);
    expect(screen.getByLabelText("Role of Mali")).toBeDisabled();
  });

  it("is read-only for plain members", async () => {
    renderApp({ route: "/members", ...members("member") });
    await screen.findByText("Somchai");
    expect(screen.queryByRole("combobox", { name: /Role of/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Remove/ })).not.toBeInTheDocument();
  });

  it("loads the next page", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/members",
      routes: {
        "GET /api/v1/workspace/members": (_b, url) =>
          url.includes("cursor=c1")
            ? { items: [somchai], next_cursor: null }
            : { items: [mali], next_cursor: "c1" },
      },
    });
    await screen.findByText("Mali");
    await user.click(screen.getByRole("button", { name: "Load more" }));
    expect(await screen.findByText("Somchai")).toBeInTheDocument();
    expect(app.calls.filter((c) => c.path.endsWith("/members"))).toHaveLength(2);
  });
});
