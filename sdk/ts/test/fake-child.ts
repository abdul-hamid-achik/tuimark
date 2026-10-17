// A stand-in for `tuimark host FILE` that speaks the SPEC §19.1 protocol on
// fd 3 (read) and fd 4 (write), so @tuimark/host can be tested without a
// terminal or the Go binary. Behavior comes from FAKE_SCENARIO (JSON); every
// argument and every line received on fd 3 is appended to FAKE_LOG (NDJSON).
//
// Scenario members, all optional:
//   exitBeforeReady  exit with this code before writing anything
//   protocol         the `protocol` in `ready` (default 1)
//   events           [{action, source?, keys?, value?}] fired one at a time,
//                    each after the previous one's reply (`seq` is 1, 2, ...)
//   burst            write `ready` and the first event in one write
//   hang             stay up after the last event until a signal or EOF on fd 3
//   reason, code     the `exit` reason (default "quit") and exit code (0)
//   noExitMessage    exit with `code` without writing `exit`
//   ignoreTerm       do not react to SIGTERM
//   dieOn            exit(7) with no `exit` message on the first message of this type
//   store            initial values for `get`
//   noise            written right after `ready`: a string raw, anything else as JSON
import { appendFileSync, readFileSync, writeSync } from "node:fs";

interface Scenario {
  exitBeforeReady?: number;
  protocol?: number;
  events?: Array<{ action: string; source?: string; keys?: Record<string, unknown>; value?: unknown }>;
  burst?: boolean;
  hang?: boolean;
  reason?: string;
  code?: number;
  noExitMessage?: boolean;
  ignoreTerm?: boolean;
  dieOn?: string;
  store?: Record<string, unknown>;
  noise?: unknown[];
}

const scenario = JSON.parse(process.env.FAKE_SCENARIO ?? "{}") as Scenario;
const logFile = process.env.FAKE_LOG;

function log(record: unknown): void {
  if (logFile) appendFileSync(logFile, `${JSON.stringify(record)}\n`);
}

function write(text: string): void {
  let rest = Buffer.from(text);
  while (rest.length > 0) {
    try {
      rest = rest.subarray(writeSync(4, rest));
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== "EAGAIN") throw error;
    }
  }
}

function send(message: object): void {
  write(`${JSON.stringify(message)}\n`);
}

const argv = process.argv.slice(2);
log({ argv });
const dataAt = argv.indexOf("--data");
if (dataAt >= 0) log({ data: JSON.parse(readFileSync(argv[dataAt + 1]!, "utf8")) });

if (scenario.exitBeforeReady !== undefined) process.exit(scenario.exitBeforeReady);

const store = new Map(Object.entries(scenario.store ?? {}));
const events = scenario.events ?? [];
let nextEvent = 0;
let waiting: number | null = null;

function eventLine(): string {
  const e = events[nextEvent++]!;
  waiting = nextEvent;
  return `${JSON.stringify({
    type: "event",
    seq: waiting,
    action: e.action,
    source: e.source ?? "",
    keys: e.keys ?? null, // Go marshals a nil map as null
    value: e.value ?? null,
  })}\n`;
}

function end(reason: string, extra: object = {}, code = scenario.code ?? 0): never {
  if (scenario.noExitMessage) process.exit(scenario.code ?? 3);
  send({ type: "exit", reason, ...extra });
  process.exit(code);
}

function advance(): void {
  if (nextEvent < events.length) write(eventLine());
  else if (!scenario.hang) end(scenario.reason ?? "quit");
}

for (const [signal, code] of [
  ["SIGTERM", 143],
  ["SIGHUP", 129],
  ["SIGINT", 130],
] as const) {
  process.on(signal, () => {
    log({ signal });
    if (signal === "SIGTERM" && scenario.ignoreTerm) return;
    end("signal", { signal }, code);
  });
}

const ready = {
  type: "ready",
  protocol: scenario.protocol ?? 1,
  version: "0.4.0",
  cols: 80,
  rows: 24,
  theme: "dark",
  catalog: [{ name: "quit", builtin: true, sources: [] }],
  diagnostics: [],
};
write(
  `${JSON.stringify(ready)}\n` +
    (scenario.noise ?? []).map((n) => `${typeof n === "string" ? n : JSON.stringify(n)}\n`).join("") +
    (scenario.burst && events.length > 0 ? eventLine() : ""),
);
if (!scenario.burst) advance();
else if (events.length === 0 && !scenario.hang) end(scenario.reason ?? "quit");

function handle(msg: Record<string, any>): void {
  log({ recv: msg });
  if (scenario.dieOn === msg.type) process.exit(7);
  const ack = (ok: boolean, extra: object = {}) =>
    send({ type: "ack", id: msg.id, ok, ...(ok ? {} : { error: "boom: refused" }), ...extra });
  switch (msg.type) {
    case "set":
    case "bind":
      if (msg.path === "malformed") send({ type: "error", error: "malformed message", id: msg.id });
      else if (msg.path === "bad") ack(false);
      else {
        store.set(msg.path, msg.value);
        ack(true);
      }
      return;
    case "batch": {
      const writes: Array<{ path: string; value: unknown }> = msg.writes;
      if (writes.some((w) => w.path === "bad")) {
        ack(false);
        return;
      }
      for (const w of writes) store.set(w.path, w.value);
      ack(true);
      return;
    }
    case "get":
      if (store.has(msg.path)) ack(true, { found: true, value: store.get(msg.path) });
      else ack(true, { found: false });
      return;
    case "reply":
      if (msg.seq !== waiting) {
        send({ type: "error", error: "no such event", seq: msg.seq });
        return;
      }
      waiting = null;
      if ("error" in msg) end("error", { error: msg.error }, 1);
      if (msg.quit === true) end("quit");
      advance();
      return;
  }
}

let buffer = "";
const decoder = new TextDecoder();
for await (const chunk of Bun.file(3).stream()) {
  buffer += decoder.decode(chunk, { stream: true });
  for (let nl = buffer.indexOf("\n"); nl >= 0; nl = buffer.indexOf("\n")) {
    const line = buffer.slice(0, nl);
    buffer = buffer.slice(nl + 1);
    if (line.trim() !== "") handle(JSON.parse(line));
  }
}
log({ eof: true });
end("eof");
