import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { createHash } from "node:crypto";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { mkdtemp, rm, stat } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
  BinaryError,
  assetName,
  cacheRoot,
  cachedBinaryPath,
  packageVersion,
  parseChecksums,
  platformTarget,
  resolveBinary,
  verifyChecksum,
} from "../src/binary.ts";

const sha = (data: string | Uint8Array) => createHash("sha256").update(data).digest("hex");

let root: string;
beforeAll(async () => {
  root = await mkdtemp(join(tmpdir(), "tuimark-binary-test-"));
});
afterAll(async () => {
  await rm(root, { recursive: true, force: true });
});

async function freshHome(): Promise<string> {
  return mkdtemp(join(root, "home-"));
}

describe("platformTarget", () => {
  test("maps Node's platform and arch to the release's os and arch", () => {
    expect(platformTarget("darwin", "arm64")).toEqual({ os: "darwin", arch: "arm64", ext: "" });
    expect(platformTarget("darwin", "x64")).toEqual({ os: "darwin", arch: "amd64", ext: "" });
    expect(platformTarget("linux", "x64")).toEqual({ os: "linux", arch: "amd64", ext: "" });
    expect(platformTarget("linux", "arm64")).toEqual({ os: "linux", arch: "arm64", ext: "" });
    expect(platformTarget("win32", "x64")).toEqual({ os: "windows", arch: "amd64", ext: ".exe" });
    expect(platformTarget("win32", "arm64")).toEqual({ os: "windows", arch: "arm64", ext: ".exe" });
  });

  test("an unsupported platform or CPU is a clear error", () => {
    for (const [platform, arch] of [
      ["freebsd", "x64"],
      ["linux", "ia32"],
      ["linux", "riscv64"],
      ["aix", "ppc64"],
    ] as const) {
      const error = (() => {
        try {
          platformTarget(platform, arch);
        } catch (e) {
          return e;
        }
      })();
      expect(error).toBeInstanceOf(BinaryError);
      expect((error as BinaryError).code).toBe("unsupported-platform");
      expect((error as BinaryError).message).toContain(`${platform}/${arch}`);
      expect((error as BinaryError).message).toContain("TUIMARK_BINARY");
    }
  });

  test("the asset names are the ones GoReleaser writes", () => {
    expect(assetName("0.4.2", platformTarget("darwin", "arm64"))).toBe("tuimark_0.4.2_darwin_arm64");
    expect(assetName("0.4.2", platformTarget("linux", "x64"))).toBe("tuimark_0.4.2_linux_amd64");
    expect(assetName("0.4.2", platformTarget("win32", "x64"))).toBe("tuimark_0.4.2_windows_amd64.exe");
  });
});

describe("cache paths", () => {
  test("Linux: $XDG_CACHE_HOME, else ~/.cache", () => {
    expect(cacheRoot({ platform: "linux", env: { XDG_CACHE_HOME: "/xdg" }, home: "/home/a" })).toBe("/xdg");
    expect(cacheRoot({ platform: "linux", env: {}, home: "/home/a" })).toBe(join("/home/a", ".cache"));
    expect(cacheRoot({ platform: "linux", env: { XDG_CACHE_HOME: "" }, home: "/home/a" })).toBe(
      join("/home/a", ".cache"),
    );
  });

  test("macOS: ~/Library/Caches, whatever XDG_CACHE_HOME says", () => {
    expect(cacheRoot({ platform: "darwin", env: { XDG_CACHE_HOME: "/xdg" }, home: "/Users/a" })).toBe(
      join("/Users/a", "Library", "Caches"),
    );
  });

  test("Windows: %LOCALAPPDATA%, else ~/AppData/Local", () => {
    expect(cacheRoot({ platform: "win32", env: { LOCALAPPDATA: "C:\\L" }, home: "C:\\U" })).toBe("C:\\L");
    expect(cacheRoot({ platform: "win32", env: {}, home: "/u" })).toBe(join("/u", "AppData", "Local"));
  });

  test("the binary is <cache>/tuimark/<version>/tuimark[.exe]", () => {
    expect(
      cachedBinaryPath("0.4.2", { platform: "linux", arch: "x64", env: { XDG_CACHE_HOME: "/c" } }),
    ).toBe(join("/c", "tuimark", "0.4.2", "tuimark"));
    expect(
      cachedBinaryPath("0.4.2", { platform: "win32", arch: "x64", env: { LOCALAPPDATA: "/L" } }),
    ).toBe(join("/L", "tuimark", "0.4.2", "tuimark.exe"));
  });
});

describe("checksums", () => {
  const data = new TextEncoder().encode("a binary");
  const listing = [
    `${sha(data)}  tuimark_1.0.0_linux_amd64`,
    `${"0".repeat(64)}  tuimark_1.0.0_linux_arm64`,
    `${sha("exe")} *tuimark_1.0.0_windows_amd64.exe`,
    "not a checksum line",
  ].join("\n");

  test("parses sha256sum lines, with or without the binary marker", () => {
    const sums = parseChecksums(listing);
    expect(sums.size).toBe(3);
    expect(sums.get("tuimark_1.0.0_windows_amd64.exe")).toBe(sha("exe"));
  });

  test("accepts the data the listing names", () => {
    expect(() => verifyChecksum(data, listing, "tuimark_1.0.0_linux_amd64")).not.toThrow();
  });

  test("rejects data that does not match", () => {
    const error = (() => {
      try {
        verifyChecksum(new TextEncoder().encode("tampered"), listing, "tuimark_1.0.0_linux_amd64");
      } catch (e) {
        return e as BinaryError;
      }
    })();
    expect(error).toBeInstanceOf(BinaryError);
    expect(error?.code).toBe("checksum");
    expect(error?.message).toContain("not run");
  });

  test("rejects a name the listing does not have", () => {
    expect(() => verifyChecksum(data, listing, "tuimark_1.0.0_darwin_arm64")).toThrow(/does not list/);
  });
});

describe("resolveBinary", () => {
  const VERSION = "9.8.7";
  const platform = "linux";
  const arch = "x64";
  const asset = `tuimark_${VERSION}_linux_amd64`;
  const body = "#!/bin/sh\necho fake tuimark\n";

  /** A local stand-in for the GitHub release, counting the requests it gets. */
  function release(files: Record<string, string | Uint8Array>) {
    const hits: string[] = [];
    const server = Bun.serve({
      port: 0,
      fetch(request) {
        const path = new URL(request.url).pathname;
        hits.push(path);
        const file = files[path];
        return file === undefined ? new Response("not found", { status: 404 }) : new Response(file);
      },
    });
    return { server, hits, url: `http://127.0.0.1:${server.port}/releases` };
  }

  function good() {
    return release({
      [`/releases/v${VERSION}/checksums.txt`]: `${sha(body)}  ${asset}\n${"1".repeat(64)}  other\n`,
      [`/releases/v${VERSION}/${asset}`]: body,
    });
  }

  test("TUIMARK_BINARY wins over a cached copy and never touches the network", async () => {
    const home = await freshHome();
    const cached = cachedBinaryPath(VERSION, { platform, arch, home, env: {} });
    await Bun.write(cached, "cached");
    const mine = join(home, "mine");
    await Bun.write(mine, "mine");
    const path = await resolveBinary({
      version: VERSION,
      platform,
      arch,
      home,
      env: { TUIMARK_BINARY: mine },
      fetch: () => Promise.reject(new Error("no network expected")),
    });
    expect(path).toBe(mine);
  });

  test("a TUIMARK_BINARY that is not a file is an error, not a fall-through", async () => {
    const home = await freshHome();
    const error = await resolveBinary({
      version: VERSION,
      platform,
      arch,
      home,
      env: { TUIMARK_BINARY: join(home, "nope") },
      fetch: () => Promise.reject(new Error("no network expected")),
    }).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(BinaryError);
    expect((error as BinaryError).code).toBe("missing-binary");
  });

  test("an empty TUIMARK_BINARY is ignored", async () => {
    const home = await freshHome();
    const cached = cachedBinaryPath(VERSION, { platform, arch, home, env: {} });
    await Bun.write(cached, "cached");
    expect(await resolveBinary({ version: VERSION, platform, arch, home, env: { TUIMARK_BINARY: "" } })).toBe(
      cached,
    );
  });

  test("a cached copy is used without a download", async () => {
    const home = await freshHome();
    const cached = cachedBinaryPath(VERSION, { platform, arch, home, env: {} });
    await Bun.write(cached, "cached");
    const path = await resolveBinary({
      version: VERSION,
      platform,
      arch,
      home,
      env: {},
      fetch: () => Promise.reject(new Error("no network expected")),
    });
    expect(path).toBe(cached);
  });

  test("downloads, verifies, and caches the binary; the second call hits the cache", async () => {
    const { server, hits, url } = good();
    try {
      const home = await freshHome();
      const lines: string[] = [];
      const options = { version: VERSION, platform, arch, home, env: {}, releaseUrl: url, log: (l: string) => lines.push(l) };

      const path = await resolveBinary(options);
      expect(path).toBe(join(home, ".cache", "tuimark", VERSION, "tuimark"));
      expect(readFileSync(path, "utf8")).toBe(body);
      if (process.platform !== "win32") expect((await stat(path)).mode & 0o777).toBe(0o755);
      expect(hits).toEqual([`/releases/v${VERSION}/checksums.txt`, `/releases/v${VERSION}/${asset}`]);
      expect(lines.join("\n")).toContain(asset);
      // no temporary file is left beside it
      expect(readdirSync(join(home, ".cache", "tuimark", VERSION))).toEqual(["tuimark"]);

      expect(await resolveBinary(options)).toBe(path);
      expect(hits.length).toBe(2);
    } finally {
      void server.stop(true);
    }
  });

  test("TUIMARK_RELEASE_URL picks the mirror", async () => {
    const { server, url } = good();
    try {
      const home = await freshHome();
      const path = await resolveBinary({ version: VERSION, platform, arch, home, env: { TUIMARK_RELEASE_URL: url } });
      expect(readFileSync(path, "utf8")).toBe(body);
    } finally {
      void server.stop(true);
    }
  });

  test("a checksum mismatch discards the download: nothing cached, nothing run", async () => {
    const { server, url } = release({
      [`/releases/v${VERSION}/checksums.txt`]: `${sha(body)}  ${asset}\n`,
      [`/releases/v${VERSION}/${asset}`]: "#!/bin/sh\necho evil\n",
    });
    try {
      const home = await freshHome();
      const error = await resolveBinary({ version: VERSION, platform, arch, home, env: {}, releaseUrl: url }).catch(
        (e: unknown) => e,
      );
      expect(error).toBeInstanceOf(BinaryError);
      expect((error as BinaryError).code).toBe("checksum");
      expect(existsSync(join(home, ".cache", "tuimark"))).toBe(false);
    } finally {
      void server.stop(true);
    }
  });

  test("a release that lists no binary for this platform is not-found", async () => {
    const { server, url } = release({
      [`/releases/v${VERSION}/checksums.txt`]: `${sha(body)}  tuimark_${VERSION}_Linux_x86_64.tar.gz\n`,
      [`/releases/v${VERSION}/${asset}`]: body,
    });
    try {
      const home = await freshHome();
      const error = await resolveBinary({ version: VERSION, platform, arch, home, env: {}, releaseUrl: url }).catch(
        (e: unknown) => e,
      );
      expect((error as BinaryError).code).toBe("not-found");
      expect(existsSync(join(home, ".cache", "tuimark"))).toBe(false);
    } finally {
      void server.stop(true);
    }
  });

  test("a missing release or asset answers 404 with a way out", async () => {
    const { server, url } = release({});
    try {
      const home = await freshHome();
      const error = await resolveBinary({ version: VERSION, platform, arch, home, env: {}, releaseUrl: url }).catch(
        (e: unknown) => e,
      );
      expect((error as BinaryError).code).toBe("not-found");
      expect((error as BinaryError).message).toContain(`v${VERSION}`);
      expect((error as BinaryError).message).toContain("TUIMARK_BINARY");
    } finally {
      void server.stop(true);
    }
  });

  test("offline is a network error that names TUIMARK_BINARY", async () => {
    const { server, url } = good();
    void server.stop(true); // the port is now closed
    const home = await freshHome();
    const error = await resolveBinary({ version: VERSION, platform, arch, home, env: {}, releaseUrl: url }).catch(
      (e: unknown) => e,
    );
    expect(error).toBeInstanceOf(BinaryError);
    expect((error as BinaryError).code).toBe("network");
    expect((error as BinaryError).message).toContain("TUIMARK_BINARY");
  });

  test("an unsupported platform fails before any request", async () => {
    const error = await resolveBinary({
      version: VERSION,
      platform: "freebsd",
      arch,
      home: await freshHome(),
      env: {},
      fetch: () => Promise.reject(new Error("no network expected")),
    }).catch((e: unknown) => e);
    expect((error as BinaryError).code).toBe("unsupported-platform");
  });

  test("the default version is the package's own", async () => {
    const version = packageVersion();
    expect(version).toMatch(/^\d+\.\d+\.\d+/);
    const home = await freshHome();
    const cached = cachedBinaryPath(version, { platform, arch, home, env: {} });
    await Bun.write(cached, "cached");
    expect(await resolveBinary({ platform, arch, home, env: {} })).toBe(cached);
  });
});
