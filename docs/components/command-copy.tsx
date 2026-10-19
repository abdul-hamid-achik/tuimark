'use client';

import { useEffect, useRef, useState } from 'react';

type CopyState = 'idle' | 'copied' | 'error';

/** A one-line terminal: the accent prompt, the command, a keycap-style copy button. */
export function CommandCopy({
  command,
  prompt = '$',
  copyable = true,
  className = '',
}: {
  command: string;
  prompt?: string;
  copyable?: boolean;
  className?: string;
}) {
  const [state, setState] = useState<CopyState>('idle');
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  useEffect(() => () => clearTimeout(timer.current), []);

  async function copy() {
    try {
      await navigator.clipboard.writeText(command);
      setState('copied');
    } catch {
      setState('error');
    }
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setState('idle'), 2200);
  }

  const label = state === 'copied' ? 'Copied' : state === 'error' ? 'Select it' : 'Copy';

  return (
    <div className={`command-copy is-${state} ${copyable ? '' : 'is-static'} ${className}`}>
      <div className="cc-text">
        {prompt && (
          <span className="cc-prompt" aria-hidden="true">
            {prompt}
          </span>
        )}
        <code>{command}</code>
      </div>
      {copyable && (
        <button type="button" onClick={copy} aria-label={`${label}: ${command}`}>
          <span aria-live="polite">{label}</span>
        </button>
      )}
    </div>
  );
}
