import { execFileSync } from 'node:child_process';

/**
 * The date of the last commit that touched any of `paths` (relative to
 * docs/), for the sitemap and the pages that are not MDX. Undefined when
 * git or the history is not there (a tarball, a shallow clone without
 * those commits): the caller leaves the date out, and the build goes on.
 */
export function gitLastModified(...paths: string[]): Date | undefined {
  try {
    const out = execFileSync('git', ['log', '-1', '--format=%aI', '--', ...paths], {
      cwd: process.cwd(),
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore'],
    }).trim();
    const date = out ? new Date(out) : undefined;
    return date && !Number.isNaN(date.getTime()) ? date : undefined;
  } catch {
    return undefined;
  }
}
