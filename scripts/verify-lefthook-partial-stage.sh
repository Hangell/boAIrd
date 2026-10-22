#!/usr/bin/env bash
# Verifies that Lefthook's gofmt hook with stage_fixed formats and re-stages only
# the staged hunks, preserving unrelated unstaged changes without adding them to
# the index. This guards the partial-staging fixes present in the pinned
# Lefthook version (>= v2.1.16).
set -euo pipefail

REPO_ROOT="$(realpath "$(dirname "${BASH_SOURCE[0]}")/..")"

workdir="$(mktemp -d "${TMPDIR:-/tmp}/boaird-lefthook.XXXXXX")"
trap 'rm -rf "$workdir"' EXIT

lefthook_bin="$workdir/lefthook"
(cd "$REPO_ROOT" && go build -o "$lefthook_bin" github.com/evilmartians/lefthook/v2)

repo="$workdir/repo"
mkdir -p "$repo"
cd "$repo"
git init -q
git config user.email "lefthook-test@example.com"
git config user.name "lefthook-test"

printf 'package main\n\nfunc main() {}\n\n// separator\nfunc helper() {}\n' > sample.go
git add sample.go
git commit -qm initial

# A partially staged file: hunk A (main) will be staged unformatted; hunk B
# (helper) stays unstaged in the working tree.
printf 'package main\n\nfunc main(){print("x")}\n\n// separator\nfunc helper() { print("y") }\n' > sample.go
cat > stage.patch <<'PATCH'
diff --git a/sample.go b/sample.go
--- a/sample.go
+++ b/sample.go
@@ -1,5 +1,5 @@
 package main

-func main() {}
+func main(){print("x")}

 // separator
PATCH
git apply --cached stage.patch

cp "$REPO_ROOT/lefthook.yml" lefthook.yml
"$lefthook_bin" run pre-commit --command gofmt --no-auto-install --no-tty >/dev/null

index="$(git show :sample.go)"
worktree="$(cat sample.go)"
unstaged="$(git --no-pager diff -- sample.go)"

fail() {
	printf 'FAIL: %s\n' "$1" >&2
	printf '%s\n' '--- index ---' >&2
	printf '%s\n' "$index" >&2
	printf '%s\n' '--- worktree ---' >&2
	printf '%s\n' "$worktree" >&2
	printf '%s\n' '--- unstaged diff ---' >&2
	printf '%s\n' "$unstaged" >&2
	exit 1
}

# The staged hunk was formatted and re-staged.
grep -q 'func main() { print("x") }' <<<"$index" || fail "staged hunk was not formatted and re-staged"

# The unstaged hunk survives in the working tree.
grep -q 'func helper() { print("y") }' <<<"$worktree" || fail "unstaged hunk was lost"

# The unstaged hunk was not silently added to the index.
if grep -q 'func helper() { print("y") }' <<<"$index"; then
	fail "unstaged hunk was silently staged"
fi

# The unstaged hunk remains unstaged; the formatted main() delta must not be.
grep -q '^+func helper() { print("y") }' <<<"$unstaged" || fail "unstaged hunk is no longer unstaged"
if grep -q '^[+-].*func main' <<<"$unstaged"; then
	fail "formatted staged change was left unstaged"
fi

printf 'Lefthook partial-staging behavior verified.\n'
