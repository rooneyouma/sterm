#!/usr/bin/env zsh
# Milestone 4 proof test. Drives a real interactive zsh through a pty.
# Run from the project root:  zsh shell/test_m4.zsh

zmodload zsh/zpty || { print "zsh/zpty module not available"; exit 1 }
zmodload zsh/datetime

SRC=${0:A:h}/sterm.zsh
T=$(mktemp -d)
pass=0 fail=0

# Safety net: never hang forever.
{ sleep 120; print "TIMEOUT: test hung"; kill -9 $$ } &!
WD=$!

finish() { zpty -d z 2>/dev/null; kill $WD 2>/dev/null; rm -rf $T }

ok()    { print -P "%F{green}PASS%f  $1"; pass=$(( pass + 1 )) }
bad()   { print -P "%F{red}FAIL%f  $1"; print -r -- "        $2"; [[ -n $3 ]] && print -r -- "        got: $3"; fail=$(( fail + 1 )) }
has()   { if [[ $2 == *"$3"* ]]; then ok "$1"; else bad "$1" "expected to contain: $3" "$2"; fi }
lacks() { if [[ $2 != *"$3"* ]]; then ok "$1"; else bad "$1" "should NOT contain: $3" "$2"; fi }

print "== 0. pre-flight =="
if zsh -n $SRC 2>/dev/null; then ok "sterm.zsh has valid syntax"; else bad "sterm.zsh syntax" "run: zsh -n shell/sterm.zsh"; fi
if (( $+commands[sterm] )); then
  ok "sterm is on PATH"
else
  bad "sterm on PATH" "run: go install ./cmd/sterm  (and add ~/go/bin to PATH)"
  finish; exit 1
fi

mkdir -p $T/home $T/zdot $T/work

# Seed one known command so the ghost has something to suggest.
HOME=$T/home sterm record --cmd="m4g-deploy --prod" --cwd=$T/work

# Config for the test shell: loads our script, plus a key that dumps our state.
cat > $T/zdot/.zshrc <<ZRC
PS1='READY> '
HISTFILE=/dev/null
stty cols 200 rows 50
m4g-dep()    { print -r -- "RAN-EXACT-\$#" >> "$T/ran" }
m4g-deploy() { print -r -- "RAN-GHOST" >> "$T/ran" }
source "$SRC"
_m4_dump() { print -r -- "BUFFER=[\$BUFFER] CURSOR=[\$CURSOR] TAIL=[\$_sterm_tail] HL=[\$_sterm_hl]" >> "$T/state" }
zle -N _m4_dump
bindkey '^X^D' _m4_dump
ZRC

zpty z env HOME=$T/home ZDOTDIR=$T/zdot TERM=xterm COLUMNS=200 LINES=50 zsh -d -i
sleep 2

drain()      { while zpty -r -t z REPLY 2>/dev/null; do :; done }
key()        { zpty -w -n z "$1"; sleep ${2:-0.4}; drain }
line()       { zpty -w z "$1"; sleep ${2:-1}; drain }
clear_line() { key $'\025' 0.3 }
state() {
  : > $T/state
  key $'\030\004' 0.2
  local i
  for i in {1..30}; do [[ -s $T/state ]] && break; sleep 0.1; done
  ST=$(<$T/state)
  ST=${ST##*$'\n'}
}

print "\n== 1. recording =="
line "cd $T/work"
line "echo m4-hello"
line "false"
line " echo m4-private"
line "sterm version"
sleep 1.5
RECENT=$(HOME=$T/home sterm recent)
has   "records a normal command"             "$RECENT" "[0]  echo m4-hello"
has   "records the exit code of a failure"   "$RECENT" "[1]  false"
lacks "skips a space-prefixed command"       "$RECENT" "m4-private"
lacks "does not record sterm's own commands" "$RECENT" "sterm version"
EXPL=$(HOME=$T/home sterm query --buffer="echo m4-hel" --cwd=$T/work --explain)
has   "records the working directory"        "$EXPL" "dir=1"

print "\n== 2. ghost text =="
key "m4g-dep"; state
has "buffer holds what was typed"            "$ST" "BUFFER=[m4g-dep] CURSOR=[7]"
has "ghost is the missing part"              "$ST" "TAIL=[loy --prod]"
has "ghost is painted dim (fg=8)"            "$ST" "fg=8"

key $'\177'; state
has "ghost updates as you edit"              "$ST" "BUFFER=[m4g-de] CURSOR=[6] TAIL=[ploy --prod]"
clear_line

key "m4g-dep"; key $'\e[C'; state
has "right arrow accepts the ghost"          "$ST" "BUFFER=[m4g-deploy --prod] CURSOR=[17]"
has "no ghost or gray left after accepting"  "$ST" "TAIL=[] HL=[]"
clear_line

key "m4g-dep"; key $'\005'; state
has "Ctrl-E accepts the ghost"               "$ST" "BUFFER=[m4g-deploy --prod] CURSOR=[17] TAIL=[] HL=[]"
clear_line

key "qzqzqz"; state
has "no ghost when nothing matches"          "$ST" "BUFFER=[qzqzqz] CURSOR=[6] TAIL=[] HL=[]"
clear_line

state
has "no ghost on an empty line"              "$ST" "BUFFER=[] CURSOR=[0] TAIL=[] HL=[]"

key "m4g-dep"; key $'\e[D'; state
has "no ghost while the cursor is mid-line"  "$ST" "CURSOR=[6] TAIL=[] HL=[]"
key $'\e[C'; state
has "right arrow just moves the cursor when nothing is shown" "$ST" "BUFFER=[m4g-dep] CURSOR=[7]"
clear_line

: > $T/ran
key "m4g-dep"; key $'\n' 1.5
RAN=$(<$T/ran)
has   "Enter runs exactly what was typed"    "$RAN" "RAN-EXACT-0"
lacks "Enter never runs the ghost text"      "$RAN" "RAN-GHOST"

print "\n== 3. speed (informational) =="
s=$EPOCHREALTIME
for i in {1..20}; do HOME=$T/home sterm query --buffer="m4g-dep" --cwd=$T/work >/dev/null; done
printf "average query time: %.1f ms (budget: 20 ms)\n" $(( (EPOCHREALTIME - s) * 1000 / 20 ))

line "exit" 0.3
finish

print "\n== result =="
print "passed: $pass   failed: $fail"
if (( fail == 0 )); then
  print -P "%F{green}MILESTONE 4 COMPLETE%f"
else
  print -P "%F{red}Some checks failed. Paste this whole output back.%f"
  exit 1
fi
