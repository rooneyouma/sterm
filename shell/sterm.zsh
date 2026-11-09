# sterm.zsh: records commands and shows ghost-text suggestions.
# Load it from ~/.zshrc:  source ~/projects/sterm/shell/sterm.zsh

if (( ! $+commands[sterm] )); then
  print -u2 "sterm: binary not found in PATH, ghost text disabled"
  return 0
fi

autoload -Uz add-zsh-hook add-zle-hook-widget

# ---------- Recording ----------
typeset -g _sterm_last_cmd=""

# Runs just before a command executes; $1 is the line as typed.
_sterm_preexec() {
  _sterm_last_cmd=$1
}

# Runs just before the next prompt is drawn; $? is still the exit code.
_sterm_precmd() {
  local exit_code=$?            # must stay the first line
  [[ -n $_sterm_last_cmd ]] || return
  local cmd=$_sterm_last_cmd
  _sterm_last_cmd=""
  [[ $cmd == sterm || $cmd == sterm\ * ]] && return   # don't record our own commands
  sterm record --cmd="$cmd" --cwd="$PWD" --exit="$exit_code" &>/dev/null &!
}

# ---------- Ghost text ----------
typeset -g _sterm_hl=""          # the highlight entry we added
typeset -g _sterm_cache_buf=""   # the last buffer we asked about
typeset -g _sterm_cache_sug=""   # and the answer we got
typeset -g _sterm_tail=""        # our own copy of the ghost (the part not yet typed)
typeset -g _sterm_tail_buf=""    # the buffer that tail belongs to

_sterm_clear() {
  POSTDISPLAY=""
  _sterm_tail=""
  _sterm_tail_buf=""
  if [[ -n $_sterm_hl ]]; then
    region_highlight=("${(@)region_highlight:#* fg=8}")
    _sterm_hl=""
  fi
}

# Runs before every redraw of the command line.
_sterm_update() {
  _sterm_clear

  # Only suggest for a non-empty, single-line buffer with the cursor at the end.
  [[ -n $BUFFER && $BUFFER != *$'\n'* && $CURSOR -eq ${#BUFFER} ]] || return

  # Only ask sterm when the text actually changed.
  if [[ $BUFFER != "$_sterm_cache_buf" ]]; then
    _sterm_cache_buf=$BUFFER
    _sterm_cache_sug=$(sterm query --buffer="$BUFFER" --cwd="$PWD" 2>/dev/null)
  fi

  local sug=$_sterm_cache_sug
  [[ -n $sug && $sug == "$BUFFER"* && $sug != "$BUFFER" && $sug != *$'\n'* ]] || return

  _sterm_tail=${sug#"$BUFFER"}
  _sterm_tail_buf=$BUFFER
  POSTDISPLAY=$_sterm_tail
  _sterm_hl="${#BUFFER} $(( ${#BUFFER} + ${#_sterm_tail} )) fg=8"
  region_highlight+=("$_sterm_hl")
}

# Runs when you press Enter: wipe the ghost so it never lingers.
_sterm_line_finish() {
  _sterm_clear
  _sterm_cache_buf=""
}

# ---------- Accepting a suggestion ----------
# We read our own variable, not POSTDISPLAY, and check it still matches the buffer.
_sterm_can_accept() {
  [[ -n $_sterm_tail && $_sterm_tail_buf == "$BUFFER" && $CURSOR -eq ${#BUFFER} ]]
}

_sterm_accept() {
  BUFFER+=$_sterm_tail
  CURSOR=${#BUFFER}
  _sterm_clear
  _sterm_cache_buf=""     # force a fresh lookup, in case a longer match exists
}

# Right arrow: accept if there's a ghost, otherwise move the cursor as usual.
_sterm_forward_char() {
  if _sterm_can_accept; then _sterm_accept; else zle .forward-char; fi
}

# End / Ctrl-E: same idea.
_sterm_end_of_line() {
  if _sterm_can_accept; then _sterm_accept; else zle .end-of-line; fi
}

# ---------- Wiring it all up ----------
add-zsh-hook preexec _sterm_preexec
add-zsh-hook precmd _sterm_precmd
# Make sure our precmd runs first, so no other hook disturbs $?
precmd_functions=(_sterm_precmd ${precmd_functions:#_sterm_precmd})

add-zle-hook-widget line-pre-redraw _sterm_update
add-zle-hook-widget line-finish _sterm_line_finish

zle -N _sterm_forward_char
zle -N _sterm_end_of_line
bindkey '^[[C' _sterm_forward_char   # right arrow
bindkey '^[OC' _sterm_forward_char   # right arrow (application mode)
bindkey '^E'   _sterm_end_of_line    # Ctrl-E
bindkey '^[[F' _sterm_end_of_line    # End
bindkey '^[OF' _sterm_end_of_line    # End (application mode)
