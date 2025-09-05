#!/usr/bin/env bash
set -euo pipefail

# Dev loop helper
# Modes:
#   here  - watch local files; on change: build + test (+ optional smoke)
#   afk   - auto-pull from origin/main; rebuild + hot-redeploy API; run smoke; loop
#   once  - single pass build + test + (optional) smoke

MODE=${1:-here}
SMOKE=${SMOKE:-true}
BASE=${BASE:-http://localhost:8081}
AUTO_ACCEPT=${AUTO_ACCEPT:-false}
# Optional: where to push auto-accept commits (default: afk branch)
AUTO_ACCEPT_TARGET=${AUTO_ACCEPT_TARGET:-}
# Streaming options
STREAM_FILE=${STREAM_FILE:-logs/stream.jsonl}
STREAM_WEBHOOK=${STREAM_WEBHOOK:-}
GH_COMMENT=${GH_COMMENT:-false}

ts() { date '+%Y-%m-%d %H:%M:%S'; }

build_test() {
  echo "[$(ts)] build..."; go build ./... >/dev/null
  echo "[$(ts)] test...";  go test ./... -count=1
}

smoke() {
  if [ "$SMOKE" != "true" ]; then return 0; fi
  echo "[$(ts)] smoke ($BASE) ..."
  BASE="$BASE" ./scripts/curl-smoke.sh || true
}

compose_hot() {
  if ! command -v docker >/dev/null || ! command -v docker compose >/dev/null; then
    return 0
  fi
  echo "[$(ts)] compose: rebuild api + force-recreate"
  docker compose up -d --build --force-recreate api >/dev/null 2>&1 || true
}

checksum() {
  # generate a cheap checksum of relevant files
  find . \
    -type f \
    \( -name '*.go' -o -name 'go.mod' -o -name 'go.sum' -o -path './openapi/*' -o -path './db/migrations/*' -o -path './internal/api/embedded/*' -o -path './static/*' \) \
    -not -path './.git/*' \
    -print0 | sort -z | xargs -0 sha1sum | sha1sum | awk '{print $1}'
}

mode_here() {
  echo "[$(ts)] devloop (here): watching for changes; CTRL+C to stop"
  prev=""
  build_test; smoke
  while sleep 1; do
    cur=$(checksum)
    if [ "$cur" != "$prev" ]; then
      prev="$cur"
      echo "\n[$(ts)] change detected → build+test"
      if build_test; then
        smoke
      fi
    fi
  done
}

mode_afk() {
  LOGDIR=logs; mkdir -p "$LOGDIR"
  echo "[$(ts)] devloop (afk): auto-pull + rebuild + smoke; logs in $LOGDIR/afk.log"
  while true; do
    changed=0
    if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
      git fetch origin main >/dev/null 2>&1 || true
      L=$(git rev-parse HEAD || echo 0)
      R=$(git rev-parse origin/main || echo 0)
      if [ "$L" != "$R" ]; then
        echo "[$(ts)] pulling latest..."
        git pull --rebase --autostash origin main || true
        changed=1
      fi
    fi
    # Always build+test; if success, redeploy api and run smoke
    {
      if build_test; then
        compose_hot
        # capture smoke output for streaming summary
        SMOKE_OUT="$LOGDIR/smoke.$(date +%s).log"; : > "$SMOKE_OUT"
        if [ "$SMOKE" = "true" ]; then BASE="$BASE" ./scripts/curl-smoke.sh > "$SMOKE_OUT" 2>&1 || true; fi
        # stream summary entry (JSONL)
        mkdir -p "$(dirname "$STREAM_FILE")"
        last_commit=$(git rev-parse --short HEAD 2>/dev/null || echo "")
        changed_num=$(git diff --name-only HEAD~1..HEAD 2>/dev/null | wc -l | awk '{print $1}')
        status="ok"
        if grep -qiE "(error|failed|panic)" "$SMOKE_OUT" >/dev/null 2>&1; then status="warn"; fi
        printf '{"ts":"%s","commit":"%s","changed":%s,"status":"%s","base":"%s"}\n' "$(ts)" "$last_commit" "${changed_num:-0}" "$status" "$BASE" >> "$STREAM_FILE"
        # optional webhook (post plain text summary)
        if [ -n "$STREAM_WEBHOOK" ]; then
          (curl -sS -X POST -H 'Content-Type: application/json' \
            --data "$(printf '{"text":"AFK update: commit %s, changed %s files, status %s (%s)"}' "$last_commit" "${changed_num:-0}" "$status" "$BASE")" \
            "$STREAM_WEBHOOK" >/dev/null 2>&1 || true) &
        fi
        # optional PR comment
        if [ "$GH_COMMENT" = "true" ] && command -v gh >/dev/null 2>&1; then
          prnum=$(gh pr view --json number --jq .number 2>/dev/null || echo "")
          if [ -n "$prnum" ]; then
            gh pr comment "$prnum" -b "AFK update: commit $last_commit, changed ${changed_num:-0} files, status $status ($BASE)" >/dev/null 2>&1 || true
          fi
        fi
        # Optionally auto-accept (commit + push) on success
        if [ "$AUTO_ACCEPT" = "true" ] && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
          br=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo main)
          tsmsg=$(date -u +%Y-%m-%dT%H:%M:%SZ)
          # Stage only tracked files + new files (avoid large binaries)
          if ! git diff --quiet || [ -n "$(git ls-files --others --exclude-standard)" ]; then
            git add -A >/dev/null 2>&1 || true
            git commit -m "afk:auto-accept: build green @ $tsmsg" >/dev/null 2>&1 || true
            if [ "$AUTO_ACCEPT_TARGET" = "main" ] || [ "$AUTO_ACCEPT_TARGET" = "$br" ]; then
              # Push directly to current branch (use with caution)
              git push origin "$br" >/dev/null 2>&1 || true
            else
              # Push to afk branch; open PR if possible
              afkbr="afk-$(hostname -s 2>/dev/null || echo host)/$br"
              git branch -M "$afkbr" >/dev/null 2>&1 || git checkout -B "$afkbr" >/dev/null 2>&1 || true
              git push -u origin "$afkbr" >/dev/null 2>&1 || true
              if command -v gh >/dev/null 2>&1; then
                gh pr create --base "$br" --head "$afkbr" -t "AFK auto-accept ($tsmsg)" -b "Automated changes from devloop (build+tests passed)." >/dev/null 2>&1 || true
                gh pr merge --auto --squash "$afkbr" >/dev/null 2>&1 || true
              fi
            fi
          fi
        fi
      fi
    } >> "$LOGDIR/afk.log" 2>&1
    sleep 60
  done
}

mode_once() { build_test; smoke; }

case "$MODE" in
  here) mode_here ;;
  afk)  mode_afk  ;;
  once) mode_once ;;
  *) echo "usage: $0 [here|afk|once]"; exit 1 ;;
esac
