#!/usr/bin/env sh
set -e

if [ -z "$1" ]; then
  echo "Usage: ./start.sh <PORT>"
  exit 1
fi

PORT=$1 docker compose up --build -d
echo "Stock Market running at http://localhost:$1"