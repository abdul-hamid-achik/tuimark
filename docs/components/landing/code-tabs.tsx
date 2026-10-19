'use client';

import { Children, useId, useRef, useState, type KeyboardEvent, type ReactNode } from 'react';

/**
 * Tabs over panels that were rendered on the server (highlighted code, a
 * frame): the client only toggles which one shows. Every panel is in the
 * HTML, so the page reads the same without JavaScript up to the first tab.
 */
export function CodeTabs({
  tabs,
  label,
  children,
  className = '',
}: {
  tabs: { name: string; note?: string }[];
  label: string;
  children: ReactNode;
  className?: string;
}) {
  const [active, setActive] = useState(0);
  const id = useId();
  const buttons = useRef<(HTMLButtonElement | null)[]>([]);
  const panels = Children.toArray(children);

  function onKeyDown(e: KeyboardEvent, i: number) {
    const step = e.key === 'ArrowRight' ? 1 : e.key === 'ArrowLeft' ? -1 : 0;
    if (!step) return;
    e.preventDefault();
    const next = (i + step + tabs.length) % tabs.length;
    setActive(next);
    buttons.current[next]?.focus();
  }

  return (
    <div className={`code-tabs ${className}`}>
      <div className="code-tabs-list" role="tablist" aria-label={label}>
        {tabs.map((t, i) => (
          <button
            key={t.name}
            ref={(el) => {
              buttons.current[i] = el;
            }}
            type="button"
            role="tab"
            id={`${id}-tab-${i}`}
            aria-controls={`${id}-panel-${i}`}
            aria-selected={i === active}
            tabIndex={i === active ? 0 : -1}
            className={i === active ? 'is-active' : undefined}
            onClick={() => setActive(i)}
            onKeyDown={(e) => onKeyDown(e, i)}
          >
            <span className="code-tabs-mark" aria-hidden="true">
              {i === active ? '▸' : ' '}
            </span>
            {t.name}
            {t.note && <small>{t.note}</small>}
          </button>
        ))}
      </div>
      {panels.map((panel, i) => (
        <div
          key={i}
          className="code-tabs-panel"
          role="tabpanel"
          id={`${id}-panel-${i}`}
          aria-labelledby={`${id}-tab-${i}`}
          hidden={i !== active}
        >
          {panel}
        </div>
      ))}
    </div>
  );
}
