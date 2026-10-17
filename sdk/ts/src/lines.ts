/**
 * Reads newline-delimited UTF-8 from a stream and calls `onLine` for each
 * non-blank line, in order. A last line with no `\n` is delivered too.
 * Resolves when the stream ends; rejects if reading fails.
 */
export async function readLines(
  stream: ReadableStream<Uint8Array>,
  onLine: (line: string) => void,
): Promise<void> {
  const reader = stream.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  try {
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
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
    reader.releaseLock();
  }
}
