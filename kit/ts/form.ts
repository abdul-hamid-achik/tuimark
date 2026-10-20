// form: per-field validation rules for the kit's k-field template
// (kit/components/field.tui shows it in use).
//
// Copy this file into your project. It depends on @thelacanians/tuimark (types) and
// on route.ts (instance, part): copy that file with this one.
//
// A form is named by the id prefix of its fields: the form "profile" owns the
// fields <use id="profile__name" template="k-field"> and
// <use id="profile__email" template="k-field"> (their inputs are
// profile__name__input and profile__email__input) and the store object
// forms.profile:
//
//   forms.profile.<field>.value   written by the input's bind
//   forms.profile.<field>.error   "" or the first failing rule's message
//   forms.profile.valid           true when no field fails a rule
//
// The component's actions are form_validate (the inputs' on:change) and
// form_submit (on:submit, the Save button, ctrl+s). One handler of each serves
// every form, routed by the form the event's source names.
import type { Host, HostEvent } from "@thelacanians/tuimark";
import { instance, part } from "./route.ts";

type Ui = Pick<Host, "batch" | "get" | "set" | "on">;

/**
 * The form and the field the source of a form's event names. A field's input
 * fires as <form>__<field>__input (the use's id, then the template's input):
 * profile__email__input is the form profile and the field email. Any other
 * source is <form>__<part>: profile__save is the form profile.
 */
function formField(source: string): { form: string; field: string } {
  const id = part(source) === "input" ? instance(source) : source;
  return { form: instance(id), field: part(id) };
}

/**
 * Checks a value and returns the message to show, or "" when it is fine.
 * Write your own as a function; the ones below cover the usual cases. Except
 * `required`, they accept an empty value, so an optional field can still have
 * a format.
 */
export type Rule = (value: string) => string;

/** Fails on an empty value or one of only spaces. */
export const required = (msg: string): Rule => (v) => (v.trim() === "" ? msg : "");

/** Fails when a non-empty value does not match `re`. */
export const pattern = (re: RegExp, msg: string): Rule => (v) => (v !== "" && !re.test(v) ? msg : "");

/** Fails when a non-empty value has fewer than `n` characters. */
export const minLen = (n: number, msg: string): Rule => (v) => (v !== "" && [...v].length < n ? msg : "");

/** Fails when a value has more than `n` characters. */
export const maxLen = (n: number, msg: string): Rule => (v) => ([...v].length > n ? msg : "");

export interface Form {
  /** Declares a field and the rules it must pass, checked in order; the first failure is its error. */
  field(name: string, ...rules: Rule[]): Form;
  /** Sets what runs when the form is submitted with every rule passing. `values` holds each field's text. */
  onSubmit(fn: (values: Record<string, string>) => unknown): Form;
  /** The first failing rule's message for a field's value, or "". */
  check(field: string, value: string): string;
  /** Writes the form's paths: each field's value (from `initial`, else ""), no errors, and valid. */
  reset(initial?: Record<string, string>): Promise<void>;
}

interface Entry {
  form: Form;
  fields: string[];
  rules: Map<string, Rule[]>;
  path: (field: string) => string;
  valid: (values: Record<string, string>) => boolean;
  values: (override?: Record<string, string>) => Promise<Record<string, string>>;
  submit: () => ((values: Record<string, string>) => unknown) | undefined;
}

/**
 * The forms of one app. add(name) declares a form (named by the id prefix of
 * its inputs), register() installs the form_validate and form_submit handlers
 * for all of them, and validate / submit are those handlers.
 */
export function createForms(ui: Ui) {
  const forms = new Map<string, Entry>();

  function add(name: string): Form {
    const fields: string[] = [];
    const rules = new Map<string, Rule[]>();
    let submit: ((values: Record<string, string>) => unknown) | undefined;
    const path = (field: string) => `forms.${name}.${field}`;

    const check = (field: string, value: string): string => {
      for (const r of rules.get(field) ?? []) {
        const msg = r(value);
        if (msg !== "") return msg;
      }
      return "";
    };
    const valid = (values: Record<string, string>) => fields.every((f) => check(f, values[f] ?? "") === "");
    const values = async (override: Record<string, string> = {}): Promise<Record<string, string>> => {
      const out: Record<string, string> = {};
      for (const f of fields) {
        const v = override[f] ?? (await ui.get(`${path(f)}.value`)).value;
        out[f] = typeof v === "string" ? v : "";
      }
      return out;
    };

    const form: Form = {
      field(field, ...rs) {
        if (!rules.has(field)) fields.push(field);
        rules.set(field, rs);
        return form;
      },
      onSubmit(fn) {
        submit = fn;
        return form;
      },
      check,
      async reset(initial = {}) {
        // A field the initial values do not mention goes back to "".
        const vs: Record<string, string> = {};
        for (const f of fields) vs[f] = initial[f] ?? "";
        await ui.batch([
          ...fields.flatMap((f) => [
            { path: `${path(f)}.value`, value: vs[f] ?? "" },
            { path: `${path(f)}.error`, value: "" },
          ]),
          { path: `forms.${name}.valid`, value: valid(vs) },
        ]);
      },
    };
    forms.set(name, { form, fields, rules, path, valid, values, submit: () => submit });
    return form;
  }

  /**
   * The form an event belongs to: the one its source names, else the only
   * form there is (ctrl+s fires with whatever has focus as its source), else
   * none.
   */
  function formFor(source: string): (Entry & { name: string }) | undefined {
    let name = formField(source).form;
    let entry = forms.get(name);
    if (!entry && forms.size === 1) [name, entry] = [...forms][0]!;
    return entry && { name, ...entry };
  }

  /**
   * The handler of form_validate: checks the field that changed (named by the
   * source: profile__email__input is the field email), shows or clears its
   * error, and updates valid.
   * Other fields' errors are left alone, so a field the user has not reached
   * yet does not turn red.
   */
  async function validate(ev: HostEvent): Promise<void> {
    const found = formFor(ev.source);
    if (!found) return; // not ours: a handler that throws ends the session
    const { name, form, rules, path, valid, values } = found;
    const { field } = formField(ev.source);
    if (!rules.has(field)) return; // an input of the form that has no rules
    const value = typeof ev.value === "string" ? ev.value : "";
    const vs = await values({ [field]: value });
    await ui.batch([
      { path: `${path(field)}.error`, value: form.check(field, value) },
      { path: `forms.${name}.valid`, value: valid(vs) },
    ]);
  }

  /**
   * The handler of form_submit: checks every field and shows every error.
   * With none, it calls the form's onSubmit. Otherwise it moves focus to the
   * first field that failed (its input, <form>__<field>__input).
   */
  async function submit(ev: HostEvent): Promise<void> {
    const found = formFor(ev.source);
    if (!found) return; // not ours (ctrl+s with focus outside any form, and several forms)
    const { name, form, fields, path, values, submit: onSubmit } = found;
    const vs = await values();
    const first = fields.find((f) => form.check(f, vs[f] ?? "") !== "");
    await ui.batch([
      ...fields.map((f) => ({ path: `${path(f)}.error`, value: form.check(f, vs[f] ?? "") })),
      { path: `forms.${name}.valid`, value: first === undefined },
    ]);
    if (first !== undefined) {
      await ui.set("@focus", `#${name}__${first}__input`);
      return;
    }
    await onSubmit()?.(vs);
  }

  /** Registers validate and submit as the handlers of form_validate and form_submit. */
  function register(): void {
    ui.on("form_validate", validate);
    ui.on("form_submit", submit);
  }

  return { add, validate, submit, register };
}

export type Forms = ReturnType<typeof createForms>;
