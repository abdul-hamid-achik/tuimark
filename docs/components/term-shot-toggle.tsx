'use client';

import { useEffect, useState } from 'react';

// The pause button of an animated TermShot. It sets data-state on the
// figure; CSS shows the moving or the still image from it. Until the reader
// chooses, their motion preference decides (see global.css), and the
// button's icon follows it once the page is hydrated. A shot rendered
// paused starts with the choice already made.
export function PlayToggle({ paused: initial = false }: { paused?: boolean }) {
  const [choice, setChoice] = useState<boolean | null>(initial ? true : null);
  const [reduced, setReduced] = useState(false);
  useEffect(() => {
    setReduced(window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  }, []);
  const paused = choice ?? reduced;

  return (
    <button
      type="button"
      className="ts-play"
      aria-pressed={paused}
      aria-label={paused ? 'Play the animation' : 'Pause the animation'}
      title={paused ? 'Play' : 'Pause'}
      onClick={(e) => {
        const next = !paused;
        setChoice(next);
        e.currentTarget.closest('figure')?.setAttribute('data-state', next ? 'paused' : 'playing');
      }}
    >
      <svg viewBox="0 0 10 10" aria-hidden="true">
        {paused ? <path d="M2.5 1.2 8.8 5 2.5 8.8z" /> : <path d="M2 1.5h2v7H2zM6 1.5h2v7H6z" />}
      </svg>
    </button>
  );
}
