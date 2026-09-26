#!/usr/bin/env bash
# Test suite for infra/ci/check-boundaries.sh.
#
# Builds temporary git repositories (mktemp) with a clean layout (expect
# pass) and with each violation (expect fail), and asserts the real
# repository passes. Run this directly with bash; it needs no test
# framework.

set -uo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
check="$script_dir/check-boundaries.sh"
repo_root="$(git -C "$script_dir" rev-parse --show-toplevel)"

pass_count=0
fail_count=0

new_git_repo() {
  local dir
  dir="$(mktemp -d)"
  git -C "$dir" init -q
  git -C "$dir" config user.email "test@example.com"
  git -C "$dir" config user.name "check-boundaries test"
  printf '%s\n' "$dir"
}

commit_all() {
  local dir="$1"
  git -C "$dir" add -A
  git -C "$dir" commit -q -m "test fixture"
}

# A minimal repository that follows every boundary rule.
clean_repo() {
  local dir="$1"
  mkdir -p "$dir/apps/eye/internal/api" "$dir/services/twin" "$dir/docs"
  cat >"$dir/apps/eye/go.mod" <<'GOMOD'
module github.com/FullFran/cordvba/apps/eye

go 1.22
GOMOD
  echo 'package api' >"$dir/apps/eye/internal/api/api.go"
  echo '# twin' >"$dir/services/twin/README.md"
  echo '# docs' >"$dir/docs/architecture.md"
  commit_all "$dir"
}

run_check() {
  bash "$check" "$1"
}

assert_pass() {
  local desc="$1" dir="$2" output
  if output="$(run_check "$dir" 2>&1)"; then
    echo "ok - $desc"
    pass_count=$((pass_count + 1))
  else
    echo "not ok - $desc (expected pass, got failure)"
    echo "$output" | sed 's/^/    /'
    fail_count=$((fail_count + 1))
  fi
}

assert_fail() {
  local desc="$1" dir="$2" output
  if output="$(run_check "$dir" 2>&1)"; then
    echo "not ok - $desc (expected failure, got pass)"
    fail_count=$((fail_count + 1))
  else
    echo "ok - $desc"
    pass_count=$((pass_count + 1))
  fi
}

d1="$(new_git_repo)"
clean_repo "$d1"
assert_pass "clean layout passes" "$d1"

d2="$(new_git_repo)"
clean_repo "$d2"
echo 'db := sqlite.Open("eye.db")' >"$d2/services/twin/notes.go"
commit_all "$d2"
assert_fail "file outside apps/eye referencing eye.db fails" "$d2"

d3="$(new_git_repo)"
clean_repo "$d3"
echo '// see apps/eye/internal/api for the shape' >"$d3/services/twin/notes.go"
commit_all "$d3"
assert_fail "file outside apps/eye referencing apps/eye/internal fails" "$d3"

d4="$(new_git_repo)"
clean_repo "$d4"
cat >>"$d4/apps/eye/go.mod" <<'GOMOD'

replace github.com/FullFran/cordvba/apps/eye => ../../other
GOMOD
commit_all "$d4"
assert_fail "go.mod replace pointing outside apps/eye fails" "$d4"

d5="$(new_git_repo)"
clean_repo "$d5"
echo '// see services/twin for the consumer' >"$d5/apps/eye/internal/api/notes.go"
commit_all "$d5"
assert_fail "apps/eye file referencing services/ fails" "$d5"

assert_pass "the real repository passes" "$repo_root"

echo
echo "$pass_count passed, $fail_count failed"
[ "$fail_count" -eq 0 ]
