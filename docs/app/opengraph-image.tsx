import { OG_SIZE, ogImage } from '@/lib/og';
import { HOME_TITLE, PITCH, TAGLINE } from '@/lib/site';

// The default social card: the landing page, and any page without its own.
export const alt = HOME_TITLE;
export const size = OG_SIZE;
export const contentType = 'image/png';

export default function Image() {
  return ogImage({ eyebrow: 'a view language for terminals', title: `${TAGLINE}.`, description: PITCH });
}
