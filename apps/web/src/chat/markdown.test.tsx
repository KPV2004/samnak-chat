import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Markdown } from "./markdown";

const names: Record<string, string> = { u1: "Mali", u2: "Somchai" };
const nameOf = (id: string) => names[id] ?? "Unknown member";

function html(text: string, me = "u1") {
  const { container } = render(<Markdown text={text} nameOf={nameOf} me={me} />);
  return container.innerHTML;
}

describe("markdown subset", () => {
  it("renders bold, italic with * or _, and inline code", () => {
    expect(html("a **bold** b")).toBe("a <strong>bold</strong> b");
    expect(html("an *italic* word")).toBe("an <em>italic</em> word");
    expect(html("an _italic_ word")).toBe("an <em>italic</em> word");
    expect(html("run `npm test` now")).toBe("run <code>npm test</code> now");
  });

  it("leaves the inside of code alone", () => {
    expect(html("`**not bold** <@u1>`")).toBe("<code>**not bold** &lt;@u1&gt;</code>");
  });

  it("does not turn snake_case or a lone star into emphasis", () => {
    expect(html("use snake_case_names here")).toBe("use snake_case_names here");
    expect(html("2 * 3 * 4")).toBe("2 * 3 * 4");
  });

  it("lets bold hold an italic part", () => {
    expect(html("**very _very_ good**")).toBe("<strong>very <em>very</em> good</strong>");
  });

  it("links [text](url) and bare http(s) URLs, opening them safely", () => {
    const out = html("see [docs](https://example.com/a?b=1) or https://example.org/x.");
    expect(out).toContain('<a href="https://example.com/a?b=1" target="_blank" rel="noopener noreferrer nofollow">docs</a>');
    // The full stop after a bare URL is not part of it.
    expect(out).toContain('<a href="https://example.org/x" target="_blank" rel="noopener noreferrer nofollow">https://example.org/x</a>.');
  });

  it("never links a javascript: or data: URL", () => {
    expect(html("[x](javascript:alert(1))")).not.toContain("<a");
    expect(html("[x](data:text/html,hi)")).not.toContain("<a");
  });

  it("shows raw HTML as text, never as elements", () => {
    const out = html('<script>alert(1)</script> <img src=x onerror=alert(1)> <b>hi</b>');
    expect(out).not.toContain("<script");
    expect(out).not.toContain("<img");
    expect(out).not.toContain("<b>");
    expect(out).toContain("&lt;script&gt;alert(1)&lt;/script&gt;");
  });

  it("renders mentions with the member's name, and marks mine", () => {
    const out = html("hi <@u2> and <@u1>");
    expect(out).toContain('<span class="mention">@Somchai</span>');
    expect(out).toContain('<span class="mention me">@Mali</span>');
  });

  it("shows an unknown mentioned user without leaking the id", () => {
    expect(html("<@u9>")).toBe('<span class="mention">@Unknown member</span>');
  });

  it("renders <!channel> as @channel", () => {
    expect(html("<!channel> standup")).toBe('<span class="mention me">@channel</span> standup');
  });

  it("keeps line breaks in the text", () => {
    expect(html("one\ntwo")).toBe("one\ntwo");
  });
});
