import type { ReactNode } from 'react';
import shots from '@/lib/generated/shots.json';
import { PlayToggle } from './term-shot-toggle';

// A screenshot rendered by docs/scripts/generate.mjs from `tuimark play`
// output. When the shot has a light variant, the one matching the site's
// theme is shown (CSS picks it; a hidden lazy image is never fetched). An
// animated shot (an example's -demo, or the hero's scripted session) plays
// in a loop, with a button that swaps it for its still last frame; a reader
// who prefers reduced motion starts on the still frame. A `paused` shot
// starts on the still frame for everyone and fetches the animation only
// when the reader presses play (for pages that show many of them).

export type Shot = {
  name: string;
  light: boolean;
  cols: number;
  rows: number;
  alt: string;
  cmd: string;
  animated?: boolean;
  ms?: number;
};

export function getShot(name: string): Shot {
  const s = (shots as Shot[]).find((x) => x.name === name);
  if (!s) throw new Error(`TermShot: no shot named "${name}" in lib/generated/shots.json`);
  return s;
}

// Must match the cell geometry in docs/scripts/svg.mjs.
export function shotSize(s: Shot) {
  return { width: Math.round(s.cols * 9.6 + 28), height: s.rows * 20 + 28 };
}

function Images({ shot, eager }: { shot: Shot; eager: boolean }) {
  const { width, height } = shotSize(shot);
  const variants: { file: string; cls: string; eager: boolean }[] = [];
  const themes = shot.light ? (['dark', 'light'] as const) : (['any'] as const);
  for (const theme of themes) {
    const sfx = theme === 'light' ? '-light' : '';
    const themeCls = theme === 'any' ? '' : theme === 'dark' ? 'ts-dark' : 'ts-light';
    if (shot.animated) {
      variants.push({ file: `${shot.name}${sfx}`, cls: `${themeCls} ts-moving`, eager: eager && theme !== 'light' });
      variants.push({ file: `${shot.name}-still${sfx}`, cls: `${themeCls} ts-still`, eager: false });
    } else {
      variants.push({ file: `${shot.name}${sfx}`, cls: themeCls, eager: eager && theme !== 'light' });
    }
  }
  return (
    <>
      {variants.map((v) => (
        // eslint-disable-next-line @next/next/no-img-element -- SVGs: nothing for next/image to optimize
        <img
          key={v.file}
          className={`ts-img ${v.cls}`}
          src={`/shots/${v.file}.svg`}
          alt={shot.alt}
          width={width}
          height={height}
          // No fetchPriority: React would turn it into a preload hint that
          // every page linking here (and so prefetching it) would follow.
          loading={v.eager ? 'eager' : 'lazy'}
          decoding="async"
        />
      ))}
    </>
  );
}

export function TermShot({
  name,
  eager = false,
  bare = false,
  paused = false,
  children,
  className = '',
}: {
  name: string;
  eager?: boolean;
  bare?: boolean;
  paused?: boolean;
  children?: ReactNode;
  className?: string;
}) {
  const shot = getShot(name);
  const startPaused = paused && !!shot.animated;
  const { width } = shotSize(shot);
  return (
    <figure
      className={`term-shot not-prose ${bare ? 'is-bare' : ''} ${className}`}
      style={bare ? undefined : { maxWidth: `${Math.round(width * 1.2)}px` }}
      data-animated={shot.animated ? '' : undefined}
      data-state={startPaused ? 'paused' : undefined}
    >
      {!bare && (
        <div className="ts-bar">
          <span className="ts-dots" aria-hidden="true">
            <i />
            <i />
            <i />
          </span>
          <code className="ts-cmd">
            <span className="ts-prompt">$</span> {shot.cmd}
          </code>
          <span className="ts-size">
            {shot.cols}×{shot.rows}
          </span>
          {shot.animated && <PlayToggle paused={startPaused} />}
        </div>
      )}
      <div className="ts-body">
        <Images shot={shot} eager={eager && !startPaused} />
        {bare && shot.animated && (
          <div className="ts-floating">
            <PlayToggle paused={startPaused} />
          </div>
        )}
      </div>
      {children && <figcaption>{children}</figcaption>}
    </figure>
  );
}
