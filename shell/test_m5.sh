#!/usr/bin/env bash
# Milestone 5 proof: daemon start/stop, query parity, freshness, crash recovery, speed.
# Run from the repo root:  bash shell/test_m5.sh
# Uses a throwaway HOME, so your real history and running daemon are untouched.
set -u

PASS=0
FAIL=0
ok()   { echo "  PASS  $1"; PASS=$((PASS+1)); }
bad()  { echo "  FAIL  $1"; FAIL=$((FAIL+1)); }

if [ ! -f go.mod ] || [ ! -d cmd/sterm ]; then
  echo "run this from the repo root (where go.mod is)"; exit 2
fi

T=$(mktemp -d /tmp/m5.XXXXXX)
BIN="$T/sterm"

cleanup() {
  "$BIN" daemon stop >/dev/null 2>&1
  pkill -9 -f "$BIN daemon run" 2>/dev/null
  rm -rf "$T"
}
trap cleanup EXIT

echo "== build"
if go build -o "$BIN" ./cmd/sterm; then ok "go build"; else bad "go build"; exit 1; fi

export HOME="$T/home" XDG_DATA_HOME="$T/home/.local/share"; mkdir -p "$HOME"
echo "== seed history"
"$BIN" record --cmd="git status" --cwd="$T"
"$BIN" record --cmd="git push origin" --cwd="$T"
sleep 1.1
"$BIN" record --cmd="git status" --cwd="$T"
"$BIN" record --cmd="git checkout main" --cwd="$T"
"$BIN" record --cmd="docker compose up" --cwd="$T"

echo "== lifecycle"
out=$("$BIN" daemon status 2>&1)
case "$out" in *"not running"*) ok "status says not running before start";; *) bad "status before start: $out";; esac

fallback=$("$BIN" query --buffer="git " --limit=3)
[ -n "$fallback" ] && ok "query works with no daemon (fallback)" || bad "query with no daemon returned nothing"

"$BIN" daemon start >/dev/null 2>&1
out=$("$BIN" daemon status 2>&1)
case "$out" in *"is running"*) ok "daemon starts and reports running";; *) bad "status after start: $out";; esac

n=$(pgrep -f "$BIN daemon run" | wc -l)
"$BIN" daemon start >/dev/null 2>&1
n2=$(pgrep -f "$BIN daemon run" | wc -l)
[ "$n" -eq 1 ] && [ "$n2" -eq 1 ] && ok "second start does not spawn a second daemon" || bad "daemon count: $n then $n2"

SOCK=$(find "$HOME" -name sterm.sock 2>/dev/null | head -1)
if [ -n "$SOCK" ]; then
  mode=$(stat -c '%a' "$SOCK")
  [ "$mode" = "600" ] && ok "socket mode is 600" || bad "socket mode is $mode"
else
  bad "socket file not found"
fi

echo "== parity: daemon vs direct"
via_daemon=$("$BIN" query --buffer="git " --limit=3)
via_db=$("$BIN" query --buffer="git " --limit=3 --direct)
if [ "$via_daemon" = "$via_db" ] && [ -n "$via_daemon" ]; then
  ok "daemon and --direct return identical results"
else
  bad "results differ"; echo "--- daemon:"; echo "$via_daemon"; echo "--- direct:"; echo "$via_db"
fi
[ "$(echo "$via_daemon" | head -1)" = "git status" ] && ok "most frequent match ranks first" || bad "top result was: $(echo "$via_daemon" | head -1)"

empty=$("$BIN" query --buffer="zzzz-nothing")
[ -z "$empty" ] && ok "no match gives empty output" || bad "unexpected output: $empty"

echo "== freshness (poller picks up rows written by another process)"
"$BIN" record --cmd="zz-fresh-check" --cwd="$T"
found=0
for _ in $(seq 1 30); do
  if [ "$("$BIN" query --buffer="zz-fresh")" = "zz-fresh-check" ]; then found=1; break; fi
  sleep 0.1
done
[ $found -eq 1 ] && ok "new command became suggestible within 3s" || bad "new command never appeared"

echo "== crash recovery (kill -9)"
pkill -9 -f "$BIN daemon run"
sleep 0.3
out=$("$BIN" query --buffer="git " --limit=1)
[ "$out" = "git status" ] && ok "query still works after daemon is killed (fallback)" || bad "after kill: '$out'"
"$BIN" daemon start >/dev/null 2>&1
out=$("$BIN" daemon status 2>&1)
case "$out" in *"is running"*) ok "daemon restarts over stale socket and lock";; *) bad "restart failed: $out";; esac
[ "$("$BIN" query --buffer="git " --limit=1)" = "git status" ] && ok "restarted daemon answers correctly" || bad "restarted daemon answer wrong"

echo "== speed (50 queries each)"
timeit() {
  local s e
  s=$(date +%s%N)
  for _ in $(seq 1 50); do "$@" >/dev/null 2>&1; done
  e=$(date +%s%N)
  echo $(( (e - s) / 50 / 1000000 ))
}
d=$(timeit "$BIN" query --buffer="git ")
r=$(timeit "$BIN" query --buffer="git " --direct)
echo "  daemon: ${d} ms/query   direct: ${r} ms/query"
[ "$d" -le 20 ] && ok "daemon path within 20ms budget" || bad "daemon path is ${d}ms (budget 20ms)"
[ "$d" -le "$r" ] && ok "daemon is not slower than direct" || bad "daemon (${d}ms) slower than direct (${r}ms)"

echo "== stop"
"$BIN" daemon stop >/dev/null 2>&1
sleep 0.3
out=$("$BIN" daemon status 2>&1)
case "$out" in *"not running"*) ok "stop shuts the daemon down";; *) bad "status after stop: $out";; esac
[ -z "$(pgrep -f "$BIN daemon run")" ] && ok "no daemon process left" || bad "daemon process still alive"
if [ -z "$SOCK" ] || [ ! -e "$SOCK" ]; then ok "socket file removed on clean shutdown"; else bad "socket file left behind"; fi

echo
echo "passed: $PASS   failed: $FAIL"
[ "$FAIL" -eq 0 ]
