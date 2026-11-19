import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import { useLocale } from "../state/locale";
import { SidebarSection } from "./SidebarSection";

function section() {
  return (
    <SidebarSection id="demo" title="Channels" badge={<span>3 unread</span>}>
      <a href="/chat/c1">general</a>
    </SidebarSection>
  );
}

describe("SidebarSection", () => {
  beforeEach(() => {
    localStorage.clear();
    useLocale.setState({ locale: "en" });
  });

  it("is open by default and folds away from its heading", async () => {
    const user = userEvent.setup();
    render(section());
    const heading = screen.getByRole("button", { name: "Channels" });
    expect(heading).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("link", { name: "general" })).toBeInTheDocument();

    await user.click(heading);
    expect(heading).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByRole("link", { name: "general" })).not.toBeInTheDocument();
  });

  it("keeps the badge on the heading only while folded", async () => {
    const user = userEvent.setup();
    render(section());
    expect(screen.queryByText("3 unread")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /Channels/ }));
    expect(screen.getByText("3 unread")).toBeInTheDocument();
  });

  it("remembers the fold for the next visit", async () => {
    const user = userEvent.setup();
    const first = render(section());
    await user.click(screen.getByRole("button", { name: "Channels" }));
    first.unmount();

    render(section());
    expect(screen.getByRole("button", { name: /Channels/ })).toHaveAttribute("aria-expanded", "false");
  });
});
