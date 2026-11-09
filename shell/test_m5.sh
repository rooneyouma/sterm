#!/usr/bin/env bash
# Milestone 5 proof test: daemon + in-memory index.
# Plain commands against a throwaway HOME. Run:  bash shell/test_m5.sh

cd "$(dirname "$0")/.." || exit 1
T=$(mktemp -d)
BIN=$T/bin/sterm
pass=0
fail=0

cleanup() {
  [[ -x $BIN ]] && HOME=$T "$BIN" daemon stop >/dev/null 2>&1
  pkill -9 -f "$BIN daemon run" 2>/dev/null
  rm -rf "$T"
}
trap cleanup EXIT

green=$'\e[32m'; red=$'\e[31m'; off=$'\e[0m'
ok()    { echo "${green}PASS${off}  $1"; pass=$((pass+1)); }
bad()   { echo "${red}FAIL${off}  $1"; [[ -n ${2:-} ]] && echo "        $2"; fail=$((fail+1)); }
has()   { if [[ $2 == *"$3"* ]]; then ok "$1"; else bad "$1" "expected to contain: $3 | got: $2"; fi; }
eq()    { if [[ $2 == "$3" ]]; then ok "$1"; else bad "$1" "expected: $3 | got: $2"; fi; }
wait_for() { local i; for i in $(seq 1 60); do "$@" && return 0; sleep 0.05; done; return 1; }
no_daemon() { ! pgrep -f "$BIN daemon run" >/dev/null; }

echo "== 1. code checks =="
OUT=$(go vet ./... 2>&1); rc=$?
if (( rc == 0 )); then ok "go vet is clean"; else bad "go vet" "$OUT"; fi

OUT=$(go test ./... 2>&1); rc=$?
if (( rc == 0 )); then ok "go test ./... passes"; else bad "go test ./..." "$OUT"; fi
for pkg in index daemon; do
  if grep -Eq "^ok .*internal/$pkg" <<<"$OUT"; then ok "internal/$pkg has passing tests"; else bad "internal/$pkg tests did not run and pass"; fi
done

OUT=$(go test -race ./internal/index ./internal/daemon 2>&1); rc=$?
if (( rc == 0 )); then ok "race detector finds no data races"
elif [[ $OUT == *cgo* || $OUT == *gcc* || $OUT == *"C compiler"* ]]; then
  echo "SKIP  race detector needs a C compiler (sudo apt install build-essential)"
else bad "race detector" "$OUT"; fi

echo "benchmark (informational):"
go test -run='^$' -bench=. ./internal/index 2>&1 | grep Benchmark

mkdir -p "$T/bin"
if go build -o "$BIN" ./cmd/sterm; then ok "binary builds from current source"; else bad "build"; exit 1; fi
if grep -q 'sterm daemon start' shell/sterm.zsh; then ok "shell script starts the daemon"; else bad "shell/sterm.zsh is missing the 'sterm daemon start' line"; fi

# From here on, everything uses a throwaway HOME. (Build and tests above used the real one.)
export HOME=$T
q() { "$BIN" query "$@"; }
HAVE_PY=0; command -v python3 >/dev/null && HAVE_PY=1

echo
echo "== 2. seed data =="
now=$(date +%s)
for i in $(seq 1 3000); do printf '#%d\ntool%d --opt\n' $((now - i*60)) $((i % 300)); done > "$T/bash_history"
echo "$("$BIN" import --format=bash --file="$T/bash_history")"

rec() { "$BIN" record --cmd="$1" --cwd="$2" --exit="$3"; }
( for i in 1 2 3;     do rec zz-test   /tmp/api 0; sleep 1; done ) &
( for i in 1 2 3 4 5; do rec zz-deploy /tmp/web 0; sleep 1; done ) &
( for i in 1 2 3;     do rec zy-solid  /tmp/api 0; sleep 1; done ) &
( for i in 1 2 3;     do rec zy-flaky  /tmp/api 1; sleep 1; done ) &
wait
DB=$(find "$T" -name history.db | head -1)
DATA=$(dirname "$DB")
SOCK=$DATA/sterm.sock

echo
echo "== 3. fallback with no daemon =="
has "status says not running" "$("$BIN" daemon status)" "not running"
eq  "query still works without a daemon (falls back to SQLite)" "$(q --buffer=zz- --cwd=/tmp/api)" "zz-test"

echo
echo "== 4. starting the daemon =="
has "daemon start succeeds" "$("$BIN" daemon start)" "daemon started"
ST=$("$BIN" daemon status)
has "status says running" "$ST" "is running"
n=$(sed -n 's/.*running, \([0-9]*\) distinct.*/\1/p' <<<"$ST")
if [[ -n $n ]] && (( n >= 300 )); then ok "index loaded the history ($n distinct commands)"; else bad "index size" "status said: $ST"; fi
if [[ -S $SOCK ]]; then ok "socket file exists"; else bad "socket file missing at $SOCK"; fi
eq "socket permissions are 600" "$(stat -c %a "$SOCK" 2>/dev/null)" "600"
eq "data folder permissions are 700" "$(stat -c %a "$DATA" 2>/dev/null)" "700"
has "second 'daemon start' is a no-op" "$("$BIN" daemon start)" "already running"
has "a second foreground daemon refuses to run" "$(timeout 3 "$BIN" daemon run 2>&1)" "already running"
eq  "exactly one daemon process exists" "$(pgrep -fc "$BIN daemon run")" "1"
if grep -q "listening on" "$DATA/daemon.log" 2>/dev/null; then ok "daemon wrote its log"; else bad "daemon.log missing or empty"; fi

echo
echo "== 5. the daemon itself answers (raw socket) =="
raw() {
  python3 - "$SOCK" "$1" <<'PY'
import socket, sys
data = b""
s = socket.socket(socket.AF_UNIX)
s.settimeout(2)
try:
    s.connect(sys.argv[1])
    s.sendall((sys.argv[2] + "\n").encode())
    while True:
        c = s.recv(4096)
        if not c:
            break
        data += c
except Exception:
    pass
print(data.decode().strip())
PY
}
if (( HAVE_PY )); then
  has "daemon answers a query over the socket" "$(raw '{"op":"query","buffer":"zz-","cwd":"/tmp/api","limit":2}')" "zz-test"
  has "unknown requests get an error, not a crash" "$(raw '{"op":"nope"}')" "unknown op"
  raw '{{{ not json' >/dev/null
  has "daemon survives garbage input" "$("$BIN" daemon status)" "is running"
else
  echo "SKIP  python3 not found, raw socket checks skipped"
fi

echo
echo "== 6. suggestions through the daemon =="
eq "from /tmp/api, the local command wins"         "$(q --buffer=zz- --cwd=/tmp/api)" "zz-test"
eq "from /tmp/web, the local command wins"         "$(q --buffer=zz- --cwd=/tmp/web)" "zz-deploy"
eq "an always-failing command loses to a working one" "$(q --buffer=zy- --cwd=/tmp/api)" "zy-solid"

total=0; mism=0
for p in tool tool1 tool12 z zz- zy- qqq; do
  for c in /tmp/api /tmp/web /nowhere; do
    d=$(q --buffer="$p" --cwd="$c" --limit=5)
    x=$(q --buffer="$p" --cwd="$c" --limit=5 --direct)
    total=$((total+1))
    if [[ $d != "$x" ]]; then mism=$((mism+1)); echo "        mismatch: prefix=[$p] cwd=[$c]"; fi
  done
done
if (( mism == 0 )); then ok "daemon and SQLite give identical top 5 on $total queries"; else bad "$mism of $total queries differ between daemon and SQLite"; fi

echo
echo "== 7. new commands reach a running daemon =="
if (( HAVE_PY )); then
  t0=$(date +%s%N)
  "$BIN" record --cmd="zz-fresh-check" --cwd=/tmp/api
  got=""
  for i in $(seq 1 80); do
    got=$(raw '{"op":"query","buffer":"zz-fresh","limit":1}')
    [[ $got == *zz-fresh-check* ]] && break
    sleep 0.05
  done
  ms=$(( ($(date +%s%N) - t0) / 1000000 ))
  if [[ $got == *zz-fresh-check* ]] && (( ms < 3000 )); then ok "new command visible to the daemon after ${ms} ms"; else bad "new command never reached the daemon" "$got"; fi
else
  echo "SKIP  needs python3"
fi

echo
echo "== 8. speed =="
avg_us() {
  local s e i
  s=$(date +%s%N)
  for i in $(seq 1 50); do "$BIN" query --buffer="tool1" --cwd=/tmp/api "$@" >/dev/null; done
  e=$(date +%s%N)
  echo $(( (e - s) / 50000 ))
}
d_us=$(avg_us); x_us=$(avg_us --direct)
d_ms=$(awk -v u="$d_us" 'BEGIN{printf "%.1f", u/1000}')
x_ms=$(awk -v u="$x_us" 'BEGIN{printf "%.1f", u/1000}')
if (( d_us < 20000 )); then ok "via daemon: ${d_ms} ms per query (budget 20 ms)"; else bad "via daemon is ${d_ms} ms, over the 20 ms budget"; fi
echo "INFO  direct SQLite: ${x_ms} ms per query, daemon: ${d_ms} ms"

echo
echo "== 9. crash recovery =="
pkill -9 -f "$BIN daemon run"
wait_for no_daemon
has "status notices the daemon is gone" "$("$BIN" daemon status)" "not running"
eq  "queries still work with the daemon dead" "$(q --buffer=zz- --cwd=/tmp/web)" "zz-deploy"
has "daemon restarts cleanly after kill -9" "$("$BIN" daemon start)" "daemon started"
has "and is serving again" "$("$BIN" daemon status)" "is running"

echo
echo "== 10. shutdown =="
has "stop command works" "$("$BIN" daemon stop)" "daemon stopped"
if wait_for no_daemon; then ok "daemon process exited"; else bad "daemon process still alive after stop"; fi
if [[ ! -S $SOCK ]]; then ok "socket file removed on shutdown"; else bad "socket file left behind after stop"; fi

"$BIN" daemon start >/dev/null
pkill -TERM -f "$BIN daemon run"
if wait_for no_daemon; then ok "daemon exits on SIGTERM"; else bad "daemon ignored SIGTERM"; fi
if [[ ! -S $SOCK ]]; then ok "socket file removed after SIGTERM"; else bad "socket file left behind after SIGTERM"; fi

echo
echo "== result =="
echo "passed: $pass   failed: $fail"
if (( fail == 0 )); then
  echo "${green}MILESTONE 5 COMPLETE${off}"
else
  echo "${red}Some checks failed. Paste this whole output back.${off}"
  exit 1
fi
