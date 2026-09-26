/**
 * Whether the visitor has asked for less motion (issue #119). Shared by
 * every component that would otherwise start a `requestAnimationFrame`
 * loop or an animated camera move — the `--duration-*` token trick in
 * `tokens.css` only reaches CSS transitions/animations, not JS-driven
 * motion, so those need this explicit check instead.
 */
export function prefersReducedMotion(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}
