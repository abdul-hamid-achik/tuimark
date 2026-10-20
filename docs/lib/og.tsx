import { readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { ImageResponse } from 'next/og';

// Social cards, rendered at build time. The fonts are read from the
// @fontsource packages (woff, which the renderer reads; it cannot read woff2).

export const OG_SIZE = { width: 1200, height: 630 };

const fontFile = (pkg: string, file: string) => readFile(join(process.cwd(), 'node_modules', '@fontsource', pkg, 'files', file));

async function fonts() {
  const [mono, monoBold, sans] = await Promise.all([
    fontFile('jetbrains-mono', 'jetbrains-mono-latin-400-normal.woff'),
    fontFile('jetbrains-mono', 'jetbrains-mono-latin-700-normal.woff'),
    fontFile('geist-sans', 'geist-sans-latin-400-normal.woff'),
  ]);
  return [
    { name: 'JetBrains Mono', data: mono, weight: 400 as const, style: 'normal' as const },
    { name: 'JetBrains Mono', data: monoBold, weight: 700 as const, style: 'normal' as const },
    { name: 'Geist', data: sans, weight: 400 as const, style: 'normal' as const },
  ];
}

const C = {
  bg: '#0d1117',
  panel: '#161b22',
  border: '#30363d',
  fg: '#e6edf3',
  muted: '#8b949e',
  accent: '#39c5cf',
  ok: '#3fb950',
};

export async function ogImage({
  eyebrow,
  title,
  description,
}: {
  eyebrow: string;
  title: string;
  description?: string;
}) {
  const titleSize = title.length > 34 ? 60 : title.length > 22 ? 70 : 84;
  return new ImageResponse(
    (
      <div
        style={{
          width: '100%',
          height: '100%',
          display: 'flex',
          flexDirection: 'column',
          backgroundColor: C.bg,
          backgroundImage: `linear-gradient(${C.panel} 1px, transparent 1px), linear-gradient(90deg, ${C.panel} 1px, transparent 1px)`,
          backgroundSize: '24px 48px',
          padding: 48,
          fontFamily: 'Geist',
        }}
      >
        <div
          style={{
            flex: 1,
            display: 'flex',
            flexDirection: 'column',
            border: `2px solid ${C.border}`,
            borderRadius: 18,
            backgroundColor: C.bg,
            padding: '0 56px 44px',
            position: 'relative',
          }}
        >
          {/* the frame's title, sitting on its top border like `border: rounded` + title */}
          <div
            style={{
              position: 'absolute',
              top: -20,
              left: 40,
              display: 'flex',
              alignItems: 'center',
              gap: 14,
              padding: '0 16px',
              backgroundColor: C.bg,
              fontFamily: 'JetBrains Mono',
              fontSize: 28,
              fontWeight: 700,
              color: C.fg,
            }}
          >
            <div style={{ width: 16, height: 30, borderRadius: 3, backgroundColor: C.accent }} />
            tuimark
          </div>
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 14,
              marginTop: 72,
              fontFamily: 'JetBrains Mono',
              fontSize: 26,
              color: C.accent,
            }}
          >
            {/* the ▸ mark of an active tab, drawn: the font subset has no such glyph */}
            <div
              style={{
                width: 0,
                height: 0,
                borderTop: '9px solid transparent',
                borderBottom: '9px solid transparent',
                borderLeft: `13px solid ${C.accent}`,
              }}
            />
            {eyebrow}
          </div>
          <div
            style={{
              display: 'flex',
              marginTop: 18,
              fontFamily: 'JetBrains Mono',
              fontWeight: 700,
              fontSize: titleSize,
              lineHeight: 1.08,
              letterSpacing: '-0.04em',
              color: C.fg,
            }}
          >
            {title}
          </div>
          {description && (
            <div
              style={{
                display: 'flex',
                marginTop: 26,
                fontSize: 30,
                lineHeight: 1.45,
                color: C.muted,
                maxWidth: 980,
              }}
            >
              {description.length > 150 ? `${description.slice(0, 147).trimEnd()}…` : description}
            </div>
          )}
          <div
            style={{
              marginTop: 'auto',
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              fontFamily: 'JetBrains Mono',
              fontSize: 24,
              color: C.muted,
            }}
          >
            <div style={{ display: 'flex' }}>terminal UIs as documents</div>
            <div style={{ display: 'flex', color: C.fg }}>tuimark.dev</div>
          </div>
        </div>
      </div>
    ),
    { ...OG_SIZE, fonts: await fonts() },
  );
}
