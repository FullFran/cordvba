/**
 * Resolves Vite's `base` from `VITE_BASE_PATH` (issue #115): default `/`
 * for the normal dev/API-served build, or a sub-path such as `/cordvba/`
 * for a GitHub Pages user-site deploy. Pure so it can be unit-tested
 * without spinning up Vite itself; `vite.config.ts` is the only caller.
 */
export function resolveBase(env: Record<string, string | undefined>): string {
  const configured = env.VITE_BASE_PATH;
  return configured && configured.length > 0 ? configured : "/";
}
