// The SHA-256 table of this package's release binaries. In a clone it is
// `undefined`; `scripts/embed-checksums.mjs` overwrites this file from the
// release's checksums.txt when the npm package is built (see release.yml), so
// the published package carries the hashes it checks downloads against.
// Never commit a generated version of this file.

export interface Embedded {
  /** The release, without a leading `v`. */
  version: string;
  /** Asset name -> lowercase hex SHA-256. */
  sha256: Record<string, string>;
}

export const embedded: Embedded | undefined = undefined;
