import kit from '@/lib/generated/kit.json';

/**
 * The kit's components, as docs/scripts/generate.mjs read them from
 * kit/components: each document's contract comment, split into its parts,
 * the document itself, and the name of its shot.
 */
export type KitComponent = {
  name: string;
  /** Its name and one-line summary, as the kit guide's table has them. */
  title: string;
  line: string;
  cls: string;
  contract: string;
  actions: string;
  keys: string;
  requires: string;
  file: string;
  data: string | null;
  shot: string;
  source: string;
};

export const KIT_GROUPS = kit as { group: string; components: KitComponent[] }[];

export const KIT_PAGE = {
  title: 'Kit components',
  description:
    'Every component of the Tuimark kit, rendered by tuimark play in the dark and light themes, with the markup to copy: app bar, status bar, tabs, cards, alerts, a data table, dialogs, a command palette, toasts, and more.',
};

/** A group's anchor on /kit: "Data and input" is #data-and-input. */
export function groupId(group: string): string {
  return group.toLowerCase().replace(/[^a-z0-9]+/g, '-');
}
