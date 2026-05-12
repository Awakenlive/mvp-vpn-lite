#!/usr/bin/env bash

IP_BIN="${IP_BIN:-ip}"
DRY_RUN="${DRY_RUN:-0}"

tun_print_command() {
  printf '+'
  local arg
  for arg in "$@"; do
    printf ' %q' "$arg"
  done
  printf '\n'
}

tun_run() {
  if [[ "$DRY_RUN" == "1" ]]; then
    tun_print_command "$@"
    return 0
  fi

  "$@"
}

tun_try() {
  if [[ "$DRY_RUN" == "1" ]]; then
    tun_print_command "$@"
    return 0
  fi

  "$@" >/dev/null 2>&1 || true
}

tun_require_root() {
  local script_name="$1"

  if [[ "$DRY_RUN" == "1" ]]; then
    return 0
  fi

  if [[ "$(id -u)" -ne 0 ]]; then
    echo "$script_name must run as root; set DRY_RUN=1 to print commands" >&2
    exit 1
  fi
}

tun_link_exists() {
  local device="$1"

  if [[ "$DRY_RUN" == "1" ]]; then
    return 1
  fi

  "$IP_BIN" link show "$device" >/dev/null 2>&1
}

tun_status_prefix() {
  if [[ "$DRY_RUN" == "1" ]]; then
    printf 'dry-run: would configure'
  else
    printf 'configured'
  fi
}

tun_cleanup_prefix() {
  if [[ "$DRY_RUN" == "1" ]]; then
    printf 'dry-run: would remove'
  else
    printf 'removed'
  fi
}
