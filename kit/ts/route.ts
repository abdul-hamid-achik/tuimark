// route: split a kit node id into its instance and its part.
//
// Copy this file into your project. It has no dependencies. dialog.ts and
// form.ts import it, so copy it with them.
//
// A kit id is "instance__part" (confirm__yes, profile__email). An action does
// not say which instance fired it, so a host routes by the instance of the
// event's source. A source with no "__" is its own instance.

/** Everything before the last "__" of `source`, or the whole id when there is none. */
export function instance(source: string): string {
  const i = source.lastIndexOf("__");
  return i >= 0 ? source.slice(0, i) : source;
}

/** Everything after the last "__" of `source`, or "" when there is none. */
export function part(source: string): string {
  const i = source.lastIndexOf("__");
  return i >= 0 ? source.slice(i + 2) : "";
}
