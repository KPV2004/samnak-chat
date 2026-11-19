import type { Message } from "@samnak/core";
import { render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it } from "vitest";
import { useLocale } from "../state/locale";
import { CardFrame, DataList, DataRow, DetailLayout, FallbackCard, Panel, Property, Properties } from "./module-ui";

beforeEach(() => useLocale.setState({ locale: "en" }));

describe("CardFrame", () => {
  it("shows the kind, the status, the title, the facts and the actions of a chat card", () => {
    render(
      <MemoryRouter>
        <CardFrame
          kind="Task"
          bar="border-l-primary"
          badge={<span>Doing</span>}
          title="Write the report"
          meta={<span>Mali</span>}
          actions={<button type="button">Comments</button>}
        />
      </MemoryRouter>,
    );
    const card = screen.getByRole("article");
    expect(card).toHaveClass("border-l-primary");
    expect(within(card).getByText("Task")).toBeInTheDocument();
    expect(within(card).getByText("Doing")).toBeInTheDocument();
    expect(within(card).getByText("Write the report")).toBeInTheDocument();
    expect(within(card).getByText("Mali")).toBeInTheDocument();
    expect(within(card).getByRole("button", { name: "Comments" })).toBeInTheDocument();
  });

  it("has a neutral bar when the card has no status colour", () => {
    render(<CardFrame kind="Card" title="Plain" />);
    expect(screen.getByRole("article")).toHaveClass("border-l-border");
  });
});

describe("FallbackCard", () => {
  const message = { fallback_text: "Leave request: 3 days", body: "" } as Message;

  it("shows the fallback text of a card this client cannot draw", () => {
    render(<FallbackCard message={message} />);
    expect(screen.getByText("Card")).toBeInTheDocument();
    expect(screen.getByText("Leave request: 3 days")).toBeInTheDocument();
  });

  it("falls back to the body when there is no fallback text", () => {
    render(<FallbackCard message={{ ...message, fallback_text: "", body: "Plain body" } as Message} />);
    expect(screen.getByText("Plain body")).toBeInTheDocument();
  });
});

describe("DataList", () => {
  it("tints only the rows that lead somewhere", () => {
    render(
      <DataList aria-label="Things">
        <DataRow interactive>Linked</DataRow>
        <DataRow>Plain</DataRow>
      </DataList>,
    );
    const rows = within(screen.getByRole("list", { name: "Things" })).getAllByRole("listitem");
    expect(rows[0]).toHaveClass("hover:bg-muted/60");
    expect(rows[1]).not.toHaveClass("hover:bg-muted/60");
  });
});

describe("DetailLayout", () => {
  it("puts the properties in a named aside next to the content", () => {
    render(
      <DetailLayout
        asideLabel="Request info"
        aside={
          <Panel title="Request info">
            <Properties>
              <Property label="Requester">Mali</Property>
            </Properties>
          </Panel>
        }
      >
        <Panel title="Details">Content</Panel>
      </DetailLayout>,
    );
    const aside = screen.getByRole("complementary", { name: "Request info" });
    expect(within(aside).getByText("Requester")).toBeInTheDocument();
    expect(within(aside).getByText("Mali")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Details" })).toBeInTheDocument();
  });
});
