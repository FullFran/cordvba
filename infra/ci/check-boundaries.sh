#!/usr/bin/env bash
# Enforces that nothing outside apps/eye/ touches eye's private store or
# internals, and that eye itself depends on nobody else in this
# repository. See docs/architecture/system-overview.md, boundary rules
# 1-3, and AGENTS.md.
#
# Usage: check-boundaries.sh [repo_root]
#   repo_root defaults to the current git repository's top level, so a
#   test can point this at a temporary repository.
#
# Exits non-zero, printing "file:line: message", on any violation:
#   (a) a tracked file outside apps/eye/ references "eye.db" or
#       "apps/eye/internal" (or the Go import path
#       github.com/FullFran/cordvba/apps/eye/internal), excluding
#       Markdown files, .git/, .atl/, odd/ and this check's own files.
#   (b) apps/eye/go.mod has a replace directive pointing outside
#       apps/eye.
#   (c) a tracked non-Markdown file under apps/eye/ references
#       services/, packages/, apps/api or apps/web paths.
#
# Only git-tracked files are scanned (git ls-files).

set -euo pipefail

repo_root="${1:-$(git rev-parse --show-toplevel)}"

self_check_path="infra/ci/check-boundaries.sh"
self_test_path="infra/ci/check-boundaries_test.sh"

# .github/labeler.yml globs eye's own directories to auto-label PRs that
# touch them. That is automation config describing paths, not a
# dependency on eye's internals, so it is exempt from rule (a).
labeler_path=".github/labeler.yml"

outside_pattern='eye\.db|apps/eye/internal|github\.com/FullFran/cordvba/apps/eye/internal'
inside_pattern='services/|packages/|apps/api|apps/web'

violations=0

while IFS= read -r file; do
  case "$file" in
  "$self_check_path" | "$self_test_path" | "$labeler_path")
    continue
    ;;
  esac

  path="$repo_root/$file"
  [ -f "$path" ] || continue

  case "$file" in
  apps/eye/*)
    case "$file" in
    *.md) continue ;;
    esac
    matches="$(grep -nE "$inside_pattern" -- "$path" 2>/dev/null || true)"
    [ -z "$matches" ] && continue
    while IFS= read -r m; do
      echo "$file:$m: apps/eye must not reference other components"
      violations=1
    done <<<"$matches"
    ;;
  *)
    case "$file" in
    *.md | .atl/* | odd/*) continue ;;
    esac
    matches="$(grep -nE "$outside_pattern" -- "$path" 2>/dev/null || true)"
    [ -z "$matches" ] && continue
    while IFS= read -r m; do
      echo "$file:$m: only apps/eye may reference eye's store or internals"
      violations=1
    done <<<"$matches"
    ;;
  esac
done < <(git -C "$repo_root" ls-files)

gomod_rel="apps/eye/go.mod"
if git -C "$repo_root" ls-files --error-unmatch "$gomod_rel" >/dev/null 2>&1; then
  gomod_path="$repo_root/$gomod_rel"
  matches="$(grep -n '=>' -- "$gomod_path" 2>/dev/null || true)"
  if [ -n "$matches" ]; then
    while IFS= read -r m; do
      line_no="${m%%:*}"
      line_text="${m#*:}"
      target="$(printf '%s' "$line_text" | sed -nE 's/.*=>[[:space:]]*([^[:space:]]+).*/\1/p')"
      case "$target" in
      ../* | /*)
        echo "$gomod_rel:$line_no: replace directive points outside apps/eye ($line_text)"
        violations=1
        ;;
      esac
    done <<<"$matches"
  fi
fi

if [ "$violations" -ne 0 ]; then
  exit 1
fi
exit 0
