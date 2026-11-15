import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { acme, errorRes, expectAlert, renderApp } from "../test-utils";

const pending = { id: "i1", email: "new@example.com", role: "member", expires_at: "2026-12-01T00:00:00Z", created_at: "2026-11-24T00:00:00Z" };

describe("Invites", () => {
  it("creates an invite and shows the link once, with a copy button", async () => {
    const user = userEvent.setup();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
    let items: unknown[] = [];
    const app = renderApp({
      route: "/invites",
      routes: {
        "GET /api/v1/workspace/invites": () => ({ items }),
        "POST /api/v1/workspace/invites": (b) => {
          items = [{ ...pending, ...b, token: undefined }];
          return { ...pending, ...b, token: "secret-token" };
        },
      },
    });

    await screen.findByText("No pending invites");
    await user.type(screen.getByLabelText("Email (optional)"), "new@example.com");
    await user.selectOptions(screen.getByLabelText("Role"), "guest");
    await user.click(screen.getByRole("button", { name: "Create invite" }));

    const link = (await screen.findByLabelText("Invite link")) as HTMLInputElement;
    expect(link.value).toBe(`${window.location.origin}/invite/secret-token`);
    expect(app.called("POST /api/v1/workspace/invites")[0]!.body).toEqual({ email: "new@example.com", role: "guest" });

    await user.click(screen.getByRole("button", { name: "Copy link" }));
    expect(writeText).toHaveBeenCalledWith(link.value);
    expect(await screen.findByText("Copied")).toBeInTheDocument();
    // the list refetches and never contains the token
    expect(await screen.findByRole("list", { name: "Pending invites" })).toBeInTheDocument();
  });

  it("omits the email for a link-only invite", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/invites",
      routes: {
        "GET /api/v1/workspace/invites": () => ({ items: [] }),
        "POST /api/v1/workspace/invites": (b) => ({ ...pending, ...b, token: "t" }),
      },
    });
    await screen.findByText("No pending invites");
    await user.click(screen.getByRole("button", { name: "Create invite" }));
    await screen.findByLabelText("Invite link");
    expect(app.called("POST /api/v1/workspace/invites")[0]!.body).toEqual({ role: "member" });
  });

  it("revokes an invite", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/invites",
      routes: {
        "GET /api/v1/workspace/invites": () => ({ items: [pending] }),
        "DELETE /api/v1/workspace/invites/i1": () => undefined,
      },
    });
    const list = await screen.findByRole("list", { name: "Pending invites" });
    expect(within(list).getByText("new@example.com")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Revoke invite for new@example.com" }));
    expect(app.called("DELETE /api/v1/workspace/invites/i1")).toHaveLength(1);
  });

  it("explains that only admins manage invites", async () => {
    const app = renderApp({
      route: "/invites",
      routes: { "GET /api/v1/workspace": () => ({ items: [{ ...acme, role: "member" }] }) },
    });
    expect(await screen.findByText("Only owners and admins can manage invites.")).toBeInTheDocument();
    expect(app.called("GET /api/v1/workspace/invites")).toHaveLength(0);
  });

  it("shows a translated error", async () => {
    renderApp({ route: "/invites", routes: { "GET /api/v1/workspace/invites": () => errorRes(403, "forbidden") } });
    await expectAlert("not allowed");
  });
});
