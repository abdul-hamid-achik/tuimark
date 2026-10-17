// A minimal parent built on @tuimark/host, run as its own process by
// process.test.ts so that signals and process.exit can be observed from
// outside. FAKE_BIN is the fake child; FIXTURE_MODE picks what the parent does.
import { spawnHost } from "../src/index.ts";

const mode = process.env.FIXTURE_MODE ?? "exitWithChild";
const host = await spawnHost("app.tui", {
  bin: process.env.FAKE_BIN,
  exitWithChild: mode === "exitWithChild",
});

// The test waits for this line before it signals the parent.
process.stdout.write("ready\n");

if (mode === "report") {
  const exit = await host.exited.catch((e: unknown) => e);
  process.stdout.write(`${JSON.stringify(exit instanceof Error ? { error: exit.message } : exit)}\n`);
  // exitWithChild is off: the parent decides, and exits normally by itself.
  process.stdout.write(`listeners ${process.listenerCount("SIGTERM")} ${process.listenerCount("SIGHUP")}\n`);
}
