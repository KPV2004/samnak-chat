import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { acme, errorRes, expectAlert, renderApp } from "../test-utils";

const beta = { id: "w2", name: "Beta", slug: "beta", role: "member" };

describe("Workspace switcher", () => {
  it("switches workspace without signing in again and sends the new header", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/members",
      routes: {
        "GET /api/v1/workspace": () => ({ items: [acme, beta] }),
        "GET /api/v1/workspace/members": () => ({ items: [], next_cursor: null }),
      },
    });
    await screen.findByRole("heading", { name: "Members" });
    await user.selectOptions(screen.getByLabelText("Workspace"), "w2");

    await screen.findByRole("heading", { name: "Members" });
    const headers = app.called("GET /api/v1/workspace/members").map((c) => c.headers["X-Workspace-ID"]);
    expect(headers).toEqual(["w1", "w2"]);
    expect(app.store.load()?.workspaceId).toBe("w2");
  });

  it("falls back to the first workspace when the stored one is gone", async () => {
    const app = renderApp({
      route: "/",
      signedIn: { workspaceId: "gone" },
      routes: { "GET /api/v1/workspace": () => ({ items: [beta] }) },
    });
    expect(await screen.findByText("Beta", { selector: "strong" })).toBeInTheDocument();
    expect(app.store.load()?.workspaceId).toBe("w2");
  });

  it("offers workspace creation when the user has none", async () => {
    renderApp({ route: "/members", signedIn: { workspaceId: null }, routes: { "GET /api/v1/workspace": () => ({ items: [] }) } });
    expect(await screen.findByText(/not in any workspace/)).toBeInTheDocument();
  });
});

describe("New workspace", () => {
  it("derives the slug, creates, and selects the new workspace", async () => {
    const user = userEvent.setup();
    const items: unknown[] = [];
    const app = renderApp({
      route: "/workspaces/new",
      signedIn: { workspaceId: null },
      routes: {
        "GET /api/v1/workspace": () => ({ items }),
        "POST /api/v1/workspace": (b) => {
          const ws = { id: "w9", role: "owner", ...b };
          items.push(ws);
          return ws;
        },
      },
    });
    await user.type(await screen.findByLabelText("Workspace name"), "My Team 2");
    expect(screen.getByLabelText("Short name (slug)")).toHaveValue("my-team-2");
    await user.click(screen.getByRole("button", { name: "Create" }));

    expect(await screen.findByText("My Team 2", { selector: "strong" })).toBeInTheDocument();
    expect(app.called("POST /api/v1/workspace")[0]!.body).toEqual({ name: "My Team 2", slug: "my-team-2" });
    expect(app.store.load()?.workspaceId).toBe("w9");
  });

  it("validates the slug and reports a taken one", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/workspaces/new",
      routes: { "POST /api/v1/workspace": () => errorRes(409, "conflict") },
    });
    await user.type(await screen.findByLabelText("Workspace name"), "Hi");
    await user.click(screen.getByRole("button", { name: "Create" }));
    expect(screen.getByLabelText("Short name (slug)")).toHaveAccessibleDescription(/3 to 40/);
    expect(app.called("POST /api/v1/workspace")).toHaveLength(0);

    await user.type(screen.getByLabelText("Workspace name"), " there");
    await user.click(screen.getByRole("button", { name: "Create" }));
    await expectAlert("already taken");
  });
});

describe("Accept invite", () => {
  it("joins with the token from the link and selects the workspace", async () => {
    const user = userEvent.setup();
    const items: unknown[] = [acme];
    const app = renderApp({
      route: "/invite/tok123",
      routes: {
        "POST /api/v1/workspace/invites/accept": () => {
          items.push(beta);
          return beta;
        },
        "GET /api/v1/workspace": () => ({ items }),
      },
    });
    await user.click(await screen.findByRole("button", { name: "Join" }));
    expect(await screen.findByText("Beta", { selector: "strong" })).toBeInTheDocument();
    expect(app.called("POST /api/v1/workspace/invites/accept")[0]!.body).toEqual({ token: "tok123" });
    expect(app.store.load()?.workspaceId).toBe("w2");
  });

  it.each([
    [404, "not_found", "invalid, expired or revoked"],
    [403, "forbidden", "different email"],
    [409, "conflict", "already a member"],
  ])("explains a %i", async (status, code, text) => {
    const user = userEvent.setup();
    renderApp({
      route: "/invite/tok",
      routes: { "POST /api/v1/workspace/invites/accept": () => errorRes(status, code) },
    });
    await user.click(await screen.findByRole("button", { name: "Join" }));
    await expectAlert(text);
  });

  it("sends a signed-out visitor to sign in and back to the invite", async () => {
    const user = userEvent.setup();
    renderApp({
      route: "/invite/tok",
      signedIn: false,
      routes: {
        "POST /api/v1/auth/login": () => ({
          access_token: "a",
          refresh_token: "r",
          expires_in: 900,
          device_id: "d1",
          user: { id: "u1", email: "me@example.com", display_name: "Mali", avatar_url: "", locale: "en", timezone: "UTC", created_at: "x" },
        }),
      },
    });
    await user.type(await screen.findByLabelText("Email"), "me@example.com");
    await user.type(screen.getByLabelText("Password"), "correct horse");
    await user.click(screen.getByRole("button", { name: "Sign in" }));
    expect(await screen.findByRole("heading", { name: "Join a workspace" })).toBeInTheDocument();
  });
});

describe("Shell", () => {
  it("signs out from the header", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/", routes: { "POST /api/v1/auth/logout": () => undefined } });
    await user.click(await screen.findByRole("button", { name: "Sign out" }));
    expect(await screen.findByRole("heading", { name: "Sign in" })).toBeInTheDocument();
    expect(app.called("POST /api/v1/auth/logout")[0]!.body).toEqual({ all_devices: false });
  });

  it("adopts the language saved on the profile", async () => {
    renderApp({ route: "/", routes: { "GET /api/v1/auth/me": () => ({ id: "u1", email: "e", display_name: "M", avatar_url: "", locale: "th", timezone: "UTC", created_at: "x" }) } });
    expect(await screen.findByRole("link", { name: "สร้าง workspace" })).toBeInTheDocument();
  });
});
