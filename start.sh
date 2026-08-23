#!/usr/bin/env bash

set -e

go build -o server ./backend/main.go
exec ./server
