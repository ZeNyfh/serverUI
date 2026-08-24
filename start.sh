#!/usr/bin/env bash

set -e

command -v tailscale >/dev/null 2>&1 || {
  echo "tailscale is required; see README.md for installation instructions." >&2
  exit 1
}

tailscale up
tailscale serve --bg --https=443 http://127.0.0.1:8080
tailscale serve --bg --https=2283 http://127.0.0.1:2283

go build -o server ./backend
exec ./server
