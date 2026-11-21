import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { errorRes, expectAlert, renderApp } from "../test-utils";

const pair = {
  access_token: "acc1",
  refresh_token: "ref1",
  expires_in: 900,
  device_id: "d9",
  user: { id: "u1", email: "me@example.com", display_name: "Mali", avatar_url: "", locale: "en", timezone: "Asia/Bangkok", created_at: "x" },
};

describe("Login", () => {
  it("redirects a signed-out visitor to the login form", async () => {
    renderApp({ route: "/", signedIn: false });
    expect(await screen.findByRole("heading", { name: "Sign in" })).toBeInTheDocument();
  });

  it("has labelled fields and signs in, then shows the app", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/login",
      signedIn: false,
      routes: { "POST /api/v1/auth/login": () => pair },
    });

    await user.type(screen.getByLabelText("Email"), "me@example.com");
    await user.type(screen.getByLabelText("Password"), "correct horse");
    await user.click(screen.getByRole("button", { name: "Sign in" }));

    expect(await screen.findByRole("navigation", { name: "Apps" })).toBeInTheDocument();
    const [login] = app.called("POST /api/v1/auth/login");
    expect(login!.body).toMatchObject({ email: "me@example.com", password: "correct horse", platform: "web" });
    expect(login!.body.device_name).toEqual(expect.any(String));
    expect(app.store.load()).toMatchObject({ accessToken: "acc1", refreshToken: "ref1", deviceId: "d9" });
  });

  it("announces a wrong password with a translated message", async () => {
    const user = userEvent.setup();
    renderApp({
      route: "/login",
      signedIn: false,
      routes: { "POST /api/v1/auth/login": () => errorRes(401, "unauthorized") },
    });
    await user.type(screen.getByLabelText("Email"), "me@example.com");
    await user.type(screen.getByLabelText("Password"), "wrong-password");
    await user.click(screen.getByRole("button", { name: "Sign in" }));
    await expectAlert("Wrong email or password");
  });

  it("tells the user to wait when rate limited", async () => {
    const user = userEvent.setup();
    renderApp({
      route: "/login",
      signedIn: false,
      routes: { "POST /api/v1/auth/login": () => errorRes(429, "rate_limited") },
    });
    await user.type(screen.getByLabelText("Email"), "me@example.com");
    await user.type(screen.getByLabelText("Password"), "wrong-password");
    await user.click(screen.getByRole("button", { name: "Sign in" }));
    await expectAlert("Too many attempts");
  });

  it("shows field errors and does not call the server when the form is empty", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/login", signedIn: false });
    await user.click(screen.getByRole("button", { name: "Sign in" }));

    const email = screen.getByLabelText("Email");
    expect(email).toBeInvalid();
    expect(email).toHaveAccessibleDescription("Enter your email");
    expect(email).toHaveFocus();
    expect(app.called("POST /api/v1/auth/login")).toHaveLength(0);
    await waitFor(() => expect(screen.getByLabelText("Password")).toBeInvalid());
  });

  it("links to the forgot-password page", () => {
    renderApp({ route: "/login", signedIn: false });
    expect(screen.getByRole("link", { name: "Forgot password?" })).toHaveAttribute("href", "/forgot-password");
  });

  it("links to registration", () => {
    renderApp({ route: "/login", signedIn: false });
    expect(screen.getByRole("link", { name: "Create an account" })).toHaveAttribute("href", "/register");
  });
});
