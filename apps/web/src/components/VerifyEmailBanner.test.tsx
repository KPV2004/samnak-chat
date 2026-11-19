import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { errorRes, me, renderApp } from "../test-utils";

const unverified = { ...me, email_verified: false };

describe("VerifyEmailBanner", () => {
  it("is not shown once the email is verified", async () => {
    renderApp({
      route: "/",
      routes: { "GET /api/v1/auth/me": () => ({ ...me, email_verified: true }) },
    });

    // The sidebar shows the profile once it has loaded, so the banner has had its chance to appear.
    expect(await screen.findByText("Mali")).toBeInTheDocument();
    expect(screen.queryByText(/Verify me@example\.com/)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Resend email" })).not.toBeInTheDocument();
  });

  it("asks an unverified user to verify, naming the address", async () => {
    renderApp({ route: "/", routes: { "GET /api/v1/auth/me": () => unverified } });

    expect(await screen.findByText("Verify me@example.com to secure your account. We sent a link to this address.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Resend email" })).toBeInTheDocument();
  });

  it("sends the verification mail again and confirms it", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/",
      routes: {
        "GET /api/v1/auth/me": () => unverified,
        "POST /api/v1/auth/email/verify/resend": () => undefined,
      },
    });

    await user.click(await screen.findByRole("button", { name: "Resend email" }));

    expect(await screen.findByText("Sent. Check your inbox.")).toBeInTheDocument();
    expect(app.called("POST /api/v1/auth/email/verify/resend")).toHaveLength(1);
  });

  it("tells the user to wait when resending too often", async () => {
    const user = userEvent.setup();
    renderApp({
      route: "/",
      routes: {
        "GET /api/v1/auth/me": () => unverified,
        "POST /api/v1/auth/email/verify/resend": () => errorRes(429, "rate_limited"),
      },
    });

    await user.click(await screen.findByRole("button", { name: "Resend email" }));

    expect(await screen.findByText("Too many attempts. Wait a moment and try again.")).toBeInTheDocument();
  });
});
