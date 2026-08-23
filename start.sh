#!/usr/bin/env bash

set -e

go build -o server ./backend
exec ./server
