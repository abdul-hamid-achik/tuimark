/**
 * Reads newline-delimited UTF-8 from a stream and calls `onLine` for each
 * non-blank line, in order. A last line with no `\n` is delivered too.
 * Resolves when the stream ends, or when `signal` aborts (the stream is
 * cancelled and the last line is dropped); rejects if reading fails.
 */
export async function readLines(
  stream: ReadableStream<Uint8Array>,
  onLine: (line: string) => void,
  signal?: AbortSignal,
): Promise<void> {
  const reader = stream.getReader();
  const abort = () => void reader.cancel().catch(() => {});
  if (signal?.aborted) abort();
  else signal?.addEventListener("abort", abort, { once: true });
  const decoder = new TextDecoder();
  let buffer = "";
  try {
    for (;;) {
      const { value, done } = await reader.read();
      if (done) {
        if (signal?.aborted) return;
        break;
      }
      const chunk = decoder.decode(value, { stream: true });
      let from = 0;
      for (let nl = chunk.indexOf("\n", from); nl >= 0; nl = chunk.indexOf("\n", from)) {
        const line = buffer + chunk.slice(from, nl);
        buffer = "";
        from = nl + 1;
        if (line.trim() !== "") onLine(line);
      }
      buffer += chunk.slice(from);
    }
    const rest = buffer + decoder.decode();
    if (rest.trim() !== "") onLine(rest);
  } finally {
    signal?.removeEventListener("abort", abort);
    reader.releaseLock();
  }
}
