import { describe, expect, it } from "vitest";
import { addPick, insertMention, mentionAt, toReadable, toTokens } from "./mentions";

const names: Record<string, string> = {
  u1: "Mali",
  u2: "Somchai",
  u3: "Mali Wong",
  u4: "สมชาย",
  u5: "tt",
  u6: "tta",
  u7: "Mali.W (ops)",
  u8: "Mali",
};
const nameOf = (id: string) => names[id] ?? "Unknown member";
const picks = (...pairs: [string, string][]) => new Map(pairs);

describe("insertMention", () => {
  it("puts the readable text and a space where the @query was", () => {
    const text = "hello @som there";
    const at = mentionAt(text, 10)!;
    expect(insertMention(text, 10, at, "@Somchai")).toEqual({ text: "hello @Somchai  there", caret: 15 });
  });
});

describe("toReadable", () => {
  it("shows each token as @name and remembers the token behind each name", () => {
    const out = toReadable("hi <@u2> and <@u1>", nameOf);
    expect(out.text).toBe("hi @Somchai and @Mali");
    expect(out.picks).toEqual(
      new Map([
        ["@Somchai", "<@u2>"],
        ["@Mali", "<@u1>"],
      ]),
    );
  });

  it("shows <!channel> as @channel", () => {
    const out = toReadable("<!channel> standup", nameOf);
    expect(out.text).toBe("@channel standup");
    expect(out.picks).toEqual(new Map([["@channel", "<!channel>"]]));
  });

  it("leaves a body without tokens as it is", () => {
    expect(toReadable("plain @Mali text", nameOf)).toEqual({ text: "plain @Mali text", picks: new Map() });
  });

  it("keeps the tokens of two people who share a name, so neither is sent as the other", () => {
    const body = "<@u1> and <@u8> both";
    const out = toReadable(body, nameOf);
    expect(out.text).toBe("<@u1> and <@u8> both");
    expect(out.picks).toEqual(new Map());
    expect(toTokens(out.text, out.picks)).toBe(body);
  });

  it("still names a person who is mentioned twice", () => {
    expect(toReadable("<@u1> and <@u1>", nameOf).text).toBe("@Mali and @Mali");
  });

  it("names an unknown user by the placeholder, and still converts back", () => {
    const out = toReadable("ping <@0198a000-0000-7000-8000-000000000000>", nameOf);
    expect(out.text).toBe("ping @Unknown member");
    expect(toTokens(out.text, out.picks)).toBe("ping <@0198a000-0000-7000-8000-000000000000>");
  });
});

describe("addPick", () => {
  it("shows the name and remembers its token", () => {
    expect(addPick(new Map(), "@Somchai", "<@u2>")).toEqual({ shown: "@Somchai", picks: new Map([["@Somchai", "<@u2>"]]) });
  });

  it("keeps one entry when the same person is picked again", () => {
    const before = picks(["@Somchai", "<@u2>"]);
    expect(addPick(before, "@Somchai", "<@u2>")).toEqual({ shown: "@Somchai", picks: new Map([["@Somchai", "<@u2>"]]) });
  });

  it("puts the token in as it is when another person already has that name", () => {
    const before = picks(["@Mali", "<@u1>"]);
    const out = addPick(before, "@Mali", "<@u8>");
    expect(out.shown).toBe("<@u8>");
    expect(out.picks).toEqual(new Map([["@Mali", "<@u1>"]]));
    expect(toTokens(`${out.shown} and @Mali`, out.picks)).toBe("<@u8> and <@u1>");
  });

  it("does not change the picks it was given", () => {
    const before = picks(["@Mali", "<@u1>"]);
    addPick(before, "@Somchai", "<@u2>");
    expect(before).toEqual(new Map([["@Mali", "<@u1>"]]));
  });
});

describe("toTokens", () => {
  it("turns a picked @name back into its token", () => {
    expect(toTokens("hello @Somchai welcome", picks(["@Somchai", "<@u2>"]))).toBe("hello <@u2> welcome");
  });

  it("turns every occurrence of a picked name", () => {
    expect(toTokens("@Somchai and @Somchai", picks(["@Somchai", "<@u2>"]))).toBe("<@u2> and <@u2>");
  });

  it("keeps a name with spaces whole", () => {
    expect(toTokens("ok @Mali Wong, thanks", picks(["@Mali Wong", "<@u3>"]))).toBe("ok <@u3>, thanks");
  });

  it("does not match a longer name: @tt is not @tta", () => {
    const p = picks(["@tt", "<@u5>"]);
    expect(toTokens("@tta", p)).toBe("@tta");
    expect(toTokens("@tt and @tta", picks(["@tt", "<@u5>"], ["@tta", "<@u6>"]))).toBe("<@u5> and <@u6>");
  });

  it("prefers the longest name when one name starts with another", () => {
    const p = picks(["@Mali", "<@u1>"], ["@Mali Wong", "<@u3>"]);
    expect(toTokens("@Mali Wong and @Mali", p)).toBe("<@u3> and <@u1>");
  });

  it("matches only at a word boundary, so an e-mail address stays", () => {
    expect(toTokens("write to mali@Somchai", picks(["@Somchai", "<@u2>"]))).toBe("write to mali@Somchai");
  });

  it("matches before punctuation and at the start of a line", () => {
    const p = picks(["@Somchai", "<@u2>"]);
    expect(toTokens("@Somchai, see\n@Somchai.", p)).toBe("<@u2>, see\n<@u2>.");
  });

  it("matches a Thai name next to a space, but not when a Thai word runs on from it", () => {
    const p = picks(["@สมชาย", "<@u4>"]);
    expect(toTokens("ขอบคุณ @สมชาย ครับ", p)).toBe("ขอบคุณ <@u4> ครับ");
    expect(toTokens("@สมชายดี", p)).toBe("@สมชายดี");
  });

  it("treats a name's characters as text, not as a pattern", () => {
    expect(toTokens("@Mali.W (ops) yes", picks(["@Mali.W (ops)", "<@u7>"]))).toBe("<@u7> yes");
    expect(toTokens("@MaliXW (ops) yes", picks(["@Mali.W (ops)", "<@u7>"]))).toBe("@MaliXW (ops) yes");
  });

  it("leaves a name that was typed and never picked as it is", () => {
    expect(toTokens("hi @Somchai", new Map())).toBe("hi @Somchai");
  });

  it("gives back the body it came from, names with spaces and Thai included", () => {
    const body = "<@u3> ask <@u4> about <!channel> and <@u2>";
    const out = toReadable(body, nameOf);
    expect(out.text).toBe("@Mali Wong ask @สมชาย about @channel and @Somchai");
    expect(toTokens(out.text, out.picks)).toBe(body);
  });
});
