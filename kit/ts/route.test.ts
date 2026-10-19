import { expect, test } from "bun:test";
import { instance, part } from "./route.ts";

test("instance and part split at the last __", () => {
  const cases: [string, string, string][] = [
    ["confirm__yes", "confirm", "yes"],
    ["profile__email", "profile", "email"],
    ["nav", "nav", ""],
    ["", "", ""],
    ["a__b__c", "a__b", "c"],
    ["toast__", "toast", ""],
    ["__x", "", "x"],
  ];
  for (const [source, inst, p] of cases) {
    expect(instance(source)).toBe(inst);
    expect(part(source)).toBe(p);
  }
});
