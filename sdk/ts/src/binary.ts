// Finds, or fetches, the `tuimark` executable that matches this package.
//
// The package version is the Tuimark release it drives: version 0.4.4 of
// @thelacanians/tuimark runs the binary of the GitHub release v0.4.4. The
// binary is looked up in this order (nothing here needs a postinstall script,
// which bun and pnpm 10 block):
//
//   1. the TUIMARK_BINARY environment variable (a path; it must exist),
//   2. the cache: <cache>/tuimark/<version>/<os>_<arch>/tuimark[.exe],
//   3. the release's raw binary tuimark_<version>_<os>_<arch>[.exe], downloaded
//      from GitHub, checked, and cached.
//
// "Checked" means: against the SHA-256 table embedded in this package
// (src/embedded.ts, written from the release's checksums.txt when the package
// is built, so the hashes travel through npm and not through GitHub). Only
// when there is no embedded table (a clone) or a mirror is in use
// (TUIMARK_RELEASE_URL) is the release's own checksums.txt fetched and used.
// A cached binary is checked against the embedded hash on every run. A binary
// that fails its check is never written to the cache and never run.
//
// Only Node APIs are used, so this runs under Node >= 18 (the `tuimark`
// launcher in bin/) as well as under Bun (the SDK).

import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { chmod, mkdir, readFile, rename, rm, stat, writeFile } from "node:fs/promises";
import { homedir } from "node:os";
import { dirname, join } from "node:path";
import { embedded as bundled, type Embedded } from "./embedded.ts";

/** The GitHub repository whose releases carry the binaries. */
export const REPOSITORY = "abdul-hamid-achik/tuimark";

/** Where release assets are served from: `<base>/v<version>/<asset>`. */
export const DEFAULT_RELEASE_URL = `https://github.com/${REPOSITORY}/releases/download`;

export type BinaryErrorCode =
  /** This operating system or CPU has no release binary. */
  | "unsupported-platform"
  /** `TUIMARK_BINARY` names a file that does not exist. */
  | "missing-binary"
  /** The release, its checksums.txt, or the binary for this platform is not there (HTTP 404). */
  | "not-found"
  /** The download failed (offline, DNS, TLS, a server error, a timeout). */
  | "network"
  /** The download does not match the release's checksums.txt. */
  | "checksum"
  /** The binary could not be written to the cache. */
  | "io";

/** Why {@link resolveBinary} could not give a binary. The message says what to do next. */
export class BinaryError extends Error {
  override name = "BinaryError";
  readonly code: BinaryErrorCode;

  constructor(code: BinaryErrorCode, message: string, options?: { cause?: unknown }) {
    super(message, options?.cause === undefined ? undefined : { cause: options.cause });
    this.code = code;
  }
}

/** The release binary for one platform, named as GoReleaser names it (.goreleaser.yaml). */
export interface Target {
  /** Go's `GOOS`. */
  os: "darwin" | "linux" | "windows";
  /** Go's `GOARCH`. */
  arch: "amd64" | "arm64";
  /** `".exe"` on Windows, else `""`. */
  ext: "" | ".exe";
}

const OSES: Record<string, Target["os"]> = { darwin: "darwin", linux: "linux", win32: "windows" };
const ARCHES: Record<string, Target["arch"]> = { x64: "amd64", arm64: "arm64" };

/**
 * Maps Node's `process.platform` and `process.arch` to the release's target.
 * Throws a {@link BinaryError} (`unsupported-platform`) for any other pair.
 */
export function platformTarget(
  platform: string = process.platform,
  arch: string = process.arch,
): Target {
  const os = OSES[platform];
  const goArch = ARCHES[arch];
  if (!os || !goArch) {
    throw new BinaryError(
      "unsupported-platform",
      `there is no tuimark release binary for ${platform}/${arch} (supported: darwin, linux, and windows on x64 and arm64). ` +
        `Build one with \`go install github.com/${REPOSITORY}/cmd/tuimark@latest\` and set TUIMARK_BINARY to its path.`,
    );
  }
  return { os, arch: goArch, ext: os === "windows" ? ".exe" : "" };
}

/** The file name of the release binary for `version` (no leading `v`) and `target`. */
export function assetName(version: string, target: Target): string {
  return `tuimark_${version}_${target.os}_${target.arch}${target.ext}`;
}

/** The inputs of the lookups below; every one defaults to the running process. */
export interface ResolveOptions {
  /** The release to use, without a leading `v`. Defaults to this package's own version. */
  version?: string;
  /** Defaults to `process.env`. */
  env?: Record<string, string | undefined>;
  /** Node's `process.platform` value. */
  platform?: string;
  /** Node's `process.arch` value. */
  arch?: string;
  /** The home directory, for the cache. Defaults to `os.homedir()`. */
  home?: string;
  /**
   * The base URL release assets are served from, as in {@link DEFAULT_RELEASE_URL}.
   * Defaults to `TUIMARK_RELEASE_URL`, then GitHub. For a mirror, or a test.
   */
  releaseUrl?: string;
  /**
   * The SHA-256 table of this release's binaries. Defaults to the one built
   * into the package (`undefined` in a clone); `null` means none, for tests.
   */
  embedded?: Embedded | null;
  /** Called with one line of progress before a download. */
  log?: (line: string) => void;
  /** A `fetch` implementation, for tests. Defaults to the global `fetch`. */
  fetch?: (url: string, init?: RequestInit) => Promise<Response>;
  /** How long one request may take, in milliseconds. Defaults to 120000. */
  timeoutMs?: number;
}

/** The directory the cache lives in: `$XDG_CACHE_HOME` or `~/.cache`, `~/Library/Caches`, `%LOCALAPPDATA%`. */
export function cacheRoot(options: ResolveOptions = {}): string {
  const env = options.env ?? process.env;
  const platform = options.platform ?? process.platform;
  const home = options.home ?? homedir();
  if (platform === "win32") {
    return nonEmpty(env.LOCALAPPDATA) ?? join(home, "AppData", "Local");
  }
  if (platform === "darwin") return join(home, "Library", "Caches");
  return nonEmpty(env.XDG_CACHE_HOME) ?? join(home, ".cache");
}

/**
 * Where the verified binary of `version` is kept:
 * `<cache>/tuimark/<version>/<os>_<arch>/tuimark[.exe]`. A binary fetched from
 * a mirror (`TUIMARK_RELEASE_URL`) gets its own directory, keyed by the mirror's
 * URL, so a normal run never picks up a mirror's download.
 */
export function cachedBinaryPath(version: string, options: ResolveOptions = {}): string {
  const target = platformTarget(options.platform, options.arch);
  const mirror = mirrorOf(options);
  const key = `${target.os}_${target.arch}${mirror === undefined ? "" : `-mirror-${createHash("sha256").update(mirror).digest("hex").slice(0, 8)}`}`;
  return join(cacheRoot(options), "tuimark", version, key, `tuimark${target.ext}`);
}

function mirrorOf(options: ResolveOptions): string | undefined {
  return options.releaseUrl ?? nonEmpty((options.env ?? process.env).TUIMARK_RELEASE_URL);
}

/**
 * This package's own version: the one built into it, else (in a clone) the
 * `version` of its package.json.
 */
export function packageVersion(): string {
  if (bundled !== undefined) return bundled.version;
  const text = readFileSync(new URL("../package.json", import.meta.url), "utf8");
  const version = (JSON.parse(text) as { version?: unknown }).version;
  if (typeof version !== "string" || version === "") {
    throw new BinaryError("io", "package.json of @thelacanians/tuimark has no version");
  }
  return version;
}

/** Parses a `sha256sum`-style file (`<hex>  <name>`, the `*` binary marker allowed) into name -> hex. */
export function parseChecksums(text: string): Map<string, string> {
  const sums = new Map<string, string>();
  for (const line of text.split(/\r?\n/)) {
    const m = /^([0-9a-fA-F]{64}) [ *]?(.+?)\s*$/.exec(line);
    if (m && m[1] && m[2]) sums.set(m[2], m[1].toLowerCase());
  }
  return sums;
}

/**
 * Checks that the SHA-256 of `data` is the one `checksums` (the text of a
 * release's checksums.txt) lists for `name`. Throws a {@link BinaryError}
 * (`checksum`, or `not-found` for a name the file does not list) otherwise.
 */
export function verifyChecksum(data: Uint8Array, checksums: string, name: string): void {
  const want = parseChecksums(checksums).get(name);
  if (want === undefined) {
    throw new BinaryError(
      "not-found",
      `checksums.txt does not list ${name}; this release has no such binary, so nothing was run`,
    );
  }
  verifySha256(data, want, name, "checksums.txt");
}

/** Checks `data` against one expected SHA-256 (hex); `source` names where `want` came from. */
export function verifySha256(data: Uint8Array, want: string, name: string, source: string): void {
  const got = sha256(data);
  if (got !== want.toLowerCase()) {
    throw new BinaryError(
      "checksum",
      `${name} does not match ${source} (expected sha256 ${want}, got ${got}); it was discarded and not run`,
    );
  }
}

function sha256(data: Uint8Array): string {
  return createHash("sha256").update(data).digest("hex");
}

/**
 * Returns the path of the `tuimark` executable for this package's release:
 * `TUIMARK_BINARY`, else the cached copy, else a verified download (see the
 * top of this file). Rejects with a {@link BinaryError}.
 */
export async function resolveBinary(options: ResolveOptions = {}): Promise<string> {
  const env = options.env ?? process.env;

  const override = nonEmpty(env.TUIMARK_BINARY);
  if (override !== undefined) {
    if (!(await isFile(override))) {
      throw new BinaryError("missing-binary", `TUIMARK_BINARY is set to ${override}, which is not a file`);
    }
    return override;
  }

  const table = options.embedded === undefined ? bundled : (options.embedded ?? undefined);
  const version = options.version ?? table?.version ?? packageVersion();
  const target = platformTarget(options.platform, options.arch);
  const name = assetName(version, target);
  const mirror = mirrorOf(options);

  // The hash this package vouches for. A mirror's binary is checked against
  // the mirror's own checksums.txt instead, and so is a clone's (no table).
  let pinned: string | undefined;
  if (mirror === undefined && table !== undefined && table.version === version) {
    pinned = table.sha256[name];
    if (pinned === undefined) {
      throw new BinaryError("not-found", `this package lists no checksum for ${name}, so nothing was run`);
    }
  }

  const cached = cachedBinaryPath(version, options);
  if (await isFile(cached)) {
    if (pinned === undefined) return cached;
    if (sha256(await readFile(cached)) === pinned) return cached;
    options.log?.(`tuimark: the cached ${cached} does not match this package's checksum; fetching it again`);
    await rm(cached, { force: true });
  }

  const base = (mirror ?? DEFAULT_RELEASE_URL).replace(/\/+$/, "");
  const url = `${base}/v${version}`;
  options.log?.(`tuimark: downloading ${name} from release v${version} (first run, cached afterwards)`);

  if (pinned !== undefined) {
    const data = await download(`${url}/${name}`, version, options, name);
    verifySha256(data, pinned, name, "the checksum built into this package");
    await store(cached, data);
    return cached;
  }
  const checksums = new TextDecoder().decode(await download(`${url}/checksums.txt`, version, options));
  const data = await download(`${url}/${name}`, version, options, name);
  verifyChecksum(data, checksums, name);
  await store(cached, data);
  return cached;
}

function nonEmpty(value: string | undefined): string | undefined {
  return value === undefined || value === "" ? undefined : value;
}

async function isFile(path: string): Promise<boolean> {
  try {
    return (await stat(path)).isFile();
  } catch {
    return false;
  }
}

async function download(
  url: string,
  version: string,
  options: ResolveOptions,
  binary?: string,
): Promise<Uint8Array> {
  const doFetch = options.fetch ?? fetch;
  let response: Response;
  try {
    response = await doFetch(url, {
      redirect: "follow",
      signal: AbortSignal.timeout(options.timeoutMs ?? 120_000),
    });
  } catch (error) {
    throw new BinaryError(
      "network",
      `cannot download ${url}: ${describe(error)}. Are you offline? Install tuimark another way ` +
        `(brew install --cask abdul-hamid-achik/tap/tuimark, or go install github.com/${REPOSITORY}/cmd/tuimark@latest) ` +
        `and set TUIMARK_BINARY to its path.`,
      { cause: error },
    );
  }
  if (response.status === 404) {
    throw new BinaryError(
      "not-found",
      binary
        ? `release v${version} has no ${binary} (${url} answered 404). Releases before the one that added raw binaries ` +
            `only ship archives; set TUIMARK_BINARY to a tuimark you installed another way.`
        : `release v${version} was not found or has no checksums.txt (${url} answered 404); ` +
            `is v${version} released? Set TUIMARK_BINARY to a tuimark you installed another way.`,
    );
  }
  if (!response.ok) {
    throw new BinaryError("network", `cannot download ${url}: HTTP ${response.status} ${response.statusText}`.trim());
  }
  try {
    return new Uint8Array(await response.arrayBuffer());
  } catch (error) {
    throw new BinaryError("network", `cannot download ${url}: ${describe(error)}`, { cause: error });
  }
}

/** Writes `data` beside `path` and renames it into place, so no reader ever sees half a binary. */
async function store(path: string, data: Uint8Array): Promise<void> {
  const tmp = `${path}.${process.pid}.${Math.random().toString(36).slice(2)}.tmp`;
  try {
    await mkdir(dirname(path), { recursive: true });
    await writeFile(tmp, data, { mode: 0o755 });
    await chmod(tmp, 0o755);
    try {
      await rename(tmp, path);
    } catch (error) {
      // Another process won the race (Windows cannot rename over a running file).
      if (!(await isFile(path))) throw error;
    }
  } catch (error) {
    throw new BinaryError("io", `cannot write ${path}: ${describe(error)}`, { cause: error });
  } finally {
    await rm(tmp, { force: true });
  }
}

function describe(error: unknown): string {
  if (error instanceof Error) {
    const cause = (error as { cause?: unknown }).cause;
    const code = cause && typeof cause === "object" ? (cause as { code?: unknown }).code : undefined;
    return typeof code === "string" ? `${error.message} (${code})` : error.message;
  }
  return String(error);
}
