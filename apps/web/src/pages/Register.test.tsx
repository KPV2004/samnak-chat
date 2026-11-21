import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { errorRes, expectAlert, me, renderApp } from "../test-utils";

const pair = { access_token: "a", refresh_token: "r", expires_in: 900, device_id: "d1", user: me };

describe("Register", () => {
  it("registers, then signs in with the same credentials", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/register",
      signedIn: false,
      routes: { "POST /api/v1/auth/register": () => ({ ...me }), "POST /api/v1/auth/login": () => pair },
    });
    await user.type(screen.getByLabelText("Display name"), "Mali");
    await user.type(screen.getByLabelText("Email"), "me@example.com");
    await user.type(screen.getByLabelText("Password"), "correct horse");
    await user.click(screen.getByRole("button", { name: "Sign up" }));

    expect(await screen.findByRole("navigation", { name: "Apps" })).toBeInTheDocument();
    expect(app.called("POST /api/v1/auth/register")[0]!.body).toEqual({
      display_name: "Mali",
      email: "me@example.com",
      password: "correct horse",
    });
    expect(app.called("POST /api/v1/auth/login")[0]!.body).toMatchObject({ email: "me@example.com", password: "correct horse" });
  });

  it("rejects a short password before calling the server", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/register", signedIn: false });
    await user.type(screen.getByLabelText("Display name"), "Mali");
    await user.type(screen.getByLabelText("Email"), "me@example.com");
    await user.type(screen.getByLabelText("Password"), "short");
    await user.click(screen.getByRole("button", { name: "Sign up" }));
    expect(screen.getByLabelText("Password")).toHaveAccessibleDescription(/must be 8 to 128 characters/);
    expect(screen.getByLabelText("Password")).toHaveFocus();
    expect(app.called("POST /api/v1/auth/register")).toHaveLength(0);
  });

  it("says when the email is taken", async () => {
    const user = userEvent.setup();
    renderApp({
      route: "/register",
      signedIn: false,
      routes: { "POST /api/v1/auth/register": () => errorRes(409, "conflict") },
    });
    await user.type(screen.getByLabelText("Display name"), "Mali");
    await user.type(screen.getByLabelText("Email"), "me@example.com");
    await user.type(screen.getByLabelText("Password"), "correct horse");
    await user.click(screen.getByRole("button", { name: "Sign up" }));
    await expectAlert("already exists");
  });
});
