import { describe, expect, test } from "bun:test";
import { component, FakeStore } from "./fake-store.ts";
import { createForms, maxLen, minLen, pattern, required, type Rule } from "./form.ts";

const email = /^[^@\s]+@[^@\s]+\.[^@\s]+$/;

async function profile(initial: Record<string, string> = {}) {
  const store = new FakeStore();
  const forms = createForms(store.host);
  const submitted: Record<string, string>[] = [];
  const form = forms
    .add("profile")
    .field("name", required("Name is required."), maxLen(20, "Keep it under 20 characters."))
    .field("email", required("Email is required."), pattern(email, "Enter a full address like name@host."))
    .onSubmit((v) => void submitted.push(v));
  forms.register();
  await form.reset(initial);
  return { store, forms, form, submitted };
}

describe("forms", () => {
  test("the paths are the ones field.tui reads, and its inputs are <form>__<field>__input", async () => {
    const { store } = await profile({ name: "Ada", email: "" });
    expect(store.at("forms.profile")).toEqual({
      name: { value: "Ada", error: "" },
      email: { value: "", error: "" },
      valid: false,
    });
    const source = component("field");
    for (const p of ["forms.profile.name.value", "forms.profile.name.error", "forms.profile.email.error"]) {
      expect(source).toContain(p);
    }
    expect(source).toContain('id="profile__name__input"');
    expect(source).toContain('id="profile__email__input"');
  });

  test("form_validate checks the field that changed", async () => {
    const { store } = await profile({ name: "Ada", email: "" });
    await store.emit("form_validate", "profile__email__input", "ada@");
    expect(store.at("forms.profile.email.error")).toBe("Enter a full address like name@host.");
    expect(store.at("forms.profile.valid")).toBe(false);
    await store.emit("form_validate", "profile__email__input", "ada@x.io");
    expect(store.at("forms.profile.email.error")).toBe("");
    expect(store.at("forms.profile.valid")).toBe(true);
  });

  test("a field the user has not reached shows no error", async () => {
    const { store } = await profile();
    await store.emit("form_validate", "profile__name__input", "A");
    expect(store.at("forms.profile.email.error")).toBe("");
    expect(store.at("forms.profile.valid")).toBe(false);
  });

  test("submit shows every error and focuses the first invalid field", async () => {
    const { store, submitted } = await profile({ name: "", email: "nope" });
    await store.emit("form_submit", "profile__save");
    expect(submitted).toEqual([]);
    expect(store.at("forms.profile.name.error")).toBe("Name is required.");
    expect(store.at("forms.profile.email.error")).toBe("Enter a full address like name@host.");
    expect(store.at("@focus")).toBe("#profile__name__input");
  });

  test("a valid submit calls onSubmit with the values (ctrl+s falls back to the only form)", async () => {
    const { store, submitted } = await profile({ name: "Ada", email: "ada@example.com" });
    await store.emit("form_submit", "somewhere_else");
    expect(submitted).toEqual([{ name: "Ada", email: "ada@example.com" }]);
    expect(store.at("forms.profile.valid")).toBe(true);
  });

  test("with two forms the source picks one", async () => {
    const store = new FakeStore();
    const forms = createForms(store.host);
    const a = forms.add("a").field("f", required("a needs f"));
    const b = forms.add("b").field("f", required("b needs f"));
    await a.reset();
    await b.reset({ f: "ok" });
    await forms.submit({ seq: 1, action: "form_submit", source: "a__save", keys: {}, value: null });
    expect(store.at("forms.a.f.error")).toBe("a needs f");
    expect(store.at("forms.b.f.error")).toBe("");
    // Not a form's own event: ignored, because a handler that throws ends the session.
    await forms.submit({ seq: 1, action: "form_submit", source: "elsewhere", keys: {}, value: null });
    await forms.validate({ seq: 1, action: "form_validate", source: "elsewhere", keys: {}, value: "x" });
    expect(store.at("forms.b.f.error")).toBe("");
  });

  test("reset sends a field the initial values do not mention back to empty", async () => {
    const { store, form } = await profile({ name: "Ada", email: "ada@example.com" });
    await store.set("forms.profile.email.error", "stale");
    await form.reset({ name: "Grace" });
    expect(store.at("forms.profile.name.value")).toBe("Grace");
    expect(store.at("forms.profile.email.value")).toBe("");
    expect(store.at("forms.profile.email.error")).toBe("");
    expect(store.at("forms.profile.valid")).toBe(false);
  });

  test("the rules", () => {
    expect(required("r")("  ")).toBe("r");
    expect(required("r")("x")).toBe("");
    expect(minLen(3, "m")("ab")).toBe("m");
    expect(minLen(3, "m")("abc")).toBe("");
    expect(minLen(3, "m")("")).toBe(""); // empty is left to required
    expect(maxLen(3, "m")("abcd")).toBe("m");
    expect(maxLen(3, "m")("héé")).toBe(""); // characters, not bytes
    expect(pattern(email, "p")("a@b")).toBe("p");
    expect(pattern(email, "p")("")).toBe("");
  });

  test("rules run in order and the first failure wins; custom rules are functions", async () => {
    const taken: Rule = (v) => (v === "admin" ? "That name is taken." : "");
    const store = new FakeStore();
    const form = createForms(store.host).add("x").field("u", required("req"), taken);
    expect(form.check("u", "")).toBe("req");
    expect(form.check("u", "admin")).toBe("That name is taken.");
    expect(form.check("u", "ada")).toBe("");
    expect(form.check("unknown", "")).toBe("");
  });
});
