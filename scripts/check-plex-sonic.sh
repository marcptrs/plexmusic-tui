#!/usr/bin/env bash
set -euo pipefail

# Simple Plex sonic detection helper script.
# Reads stored token and checks server root + recentlyAdded + album children
# Requires: jq, xmllint (for XML display), curl

CONFIG=~/.config/plexmusic-tui/config.json
if [ ! -f "$CONFIG" ]; then
  echo "No config file found at $CONFIG; sign in using the TUI first to persist a token."
  exit 1
fi

TOKEN=$(jq -r '.authToken // empty' "$CONFIG")
if [ -z "$TOKEN" ]; then
  echo "No authToken found in $CONFIG; sign in via the TUI and try again."
  exit 1
fi

# Defaults to plex.peters.casa:443 for this user's environment — update if needed
HOST="plex.peters.casa"
PORT="443"
PROTO="https"

# Allow use of -k by setting SKIP_TLS=1 environment variable
CURL_ARGS=""
if [ "${SKIP_TLS:-0}" = "1" ]; then
  CURL_ARGS="-k"
fi

# Check that required tools are present
if ! command -v jq >/dev/null 2>&1; then
  echo "jq not installed — please install jq to run this script"
  exit 1
fi
if ! command -v xmllint >/dev/null 2>&1; then
  echo "xmllint not installed — please install libxml2-utils (or equivalent) to format XML output"
  # Not fatal — we'll continue without xml format
fi

echo "Using token from: $CONFIG (redacted)"
echo "Server: $PROTO://$HOST:$PORT"

# 1. Server root (XML) - show a short preview
echo
echo "1) Server root (XML summary)"
if command -v xmllint >/dev/null 2>&1; then
  curl $CURL_ARGS -s -H "X-Plex-Token: $TOKEN" "$PROTO://$HOST:$PORT/" | xmllint --format - 2>/dev/null | sed -n '1,80p' || true
else
  curl $CURL_ARGS -s -H "X-Plex-Token: $TOKEN" "$PROTO://$HOST:$PORT/" | sed -n '1,4p' || true
fi

# 2. Recently added (album/track) sample
echo
echo "2) Recently added (type=10) — sample (first 100 items):"
RECENT_URL="$PROTO://$HOST:$PORT/library/recentlyAdded?type=10&X-Plex-Container-Size=100"
JSON_HEADER='-H "Accept: application/json"'

curl $CURL_ARGS -s -H "X-Plex-Token: $TOKEN" -H "Accept: application/json" "$RECENT_URL" | jq '.MediaContainer.Metadata[] | {title: .title, key: .key, type: (.type // null), hasSonicAnalysis: (.hasSonicAnalysis // null), musicAnalysisVersion: (.musicAnalysisVersion // null)}' || true

# 3) If the sample contains album entries with '/children', fetch and show sonic fields for the children
echo
echo "3) For any album entries showing '/children', attempt to fetch children and show sonic fields (if any):"
curl $CURL_ARGS -s -H "X-Plex-Token: $TOKEN" -H "Accept: application/json" "$RECENT_URL" | jq -r '.MediaContainer.Metadata[] | select(.key|test("/children")) | .key' | while read -r childkey; do
  if [ -z "$childkey" ]; then
    continue
  fi
  if [[ "$childkey" == http* ]]; then
    url="$childkey"
  else
    url="$PROTO://$HOST:$PORT${childkey}"
  fi
  echo "Checking: $url"
  curl $CURL_ARGS -s -H "X-Plex-Token: $TOKEN" -H "Accept: application/json" "$url" | jq '.Metadata[] | {title: .title, hasSonicAnalysis: (.hasSonicAnalysis // null), musicAnalysisVersion: (.musicAnalysisVersion // null)}' || true
done

# 4) Optionally, check libraries individually for recently added
echo
 echo "4) List music libraries and sample their recently added for analysis fields (per-library scan):"
LIBS_URL="$PROTO://$HOST:$PORT/library/sections?type=8"
LIB_KEYS=$(curl $CURL_ARGS -s -H "X-Plex-Token: $TOKEN" -H "Accept: application/json" "$LIBS_URL" | jq -r '.Directory[] | .key') || true
for lk in $LIB_KEYS; do
  # If key is absolute/hosted, extract the last path part (library key)
  cleanLK=$(basename "$lk")
  url="$PROTO://$HOST:$PORT/library/sections/${cleanLK}/recentlyAdded?type=10&X-Plex-Container-Size=20"
  echo "Library $cleanLK recent (sample):"
  curl $CURL_ARGS -s -H "X-Plex-Token: $TOKEN" -H "Accept: application/json" "$url" | jq '.MediaContainer.Metadata[] | {title: .title, key: .key, hasSonicAnalysis: (.hasSonicAnalysis // null), musicAnalysisVersion: (.musicAnalysisVersion // null)}' || true
done

echo "\nDone. If you want to rerun with insecure TLS (skipping cert verification), rerun with SKIP_TLS=1 ./scripts/check-plex-sonic.sh"
