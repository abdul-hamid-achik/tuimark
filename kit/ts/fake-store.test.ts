import { describe, expect, test } from "bun:test";
import { expandUses } from "./fake-store.ts";

describe("expandUses", () => {
  test("a prop value is placed literally, $& and $1 included", () => {
    const out = expandUses('<use template="k-card" title="a $& b $1 c $$ d"><text>x</text></use>', "t.tui");
    expect(out).toContain('title="a $& b $1 c $$ d"');
  });

  test("a prop the use leaves out removes its attribute; the content lands in the slot", () => {
    const out = expandUses('<use template="k-card"><text>body</text></use>', "t.tui");
    expect(out).not.toContain("title=");
    expect(out).not.toContain("style=");
    expect(out).toContain("<text>body</text>");
  });

  test("a use of a template the kit does not have is an error", () => {
    expect(() => expandUses('<use template="k-nope"/>', "t.tui")).toThrow("t.tui");
  });
});
