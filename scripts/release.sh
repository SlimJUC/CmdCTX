#!/usr/bin/env bash
# Internal helper: create a GitHub release and upload pre-built binaries.
# Usage:  bash scripts/release.sh <GH_TOKEN> <TAG>
# Example: bash scripts/release.sh github_pat_xxx v0.1.0

set -euo pipefail

GH_TOKEN="${1:?Usage: $0 <GH_TOKEN> <TAG>}"
TAG="${2:?Usage: $0 <GH_TOKEN> <TAG>}"
REPO="SlimJUC/CmdCTX"
API="https://api.github.com/repos/${REPO}"
DIST="dist"

# ── release body ──────────────────────────────────────────────────────────────
BODY="## cmdctx ${TAG}

Local AI-powered terminal assistant: natural language → safe shell commands.

### One-line install

\`\`\`bash
curl -fsSL https://raw.githubusercontent.com/SlimJUC/CmdCTX/main/scripts/install.sh | bash
\`\`\`

Installer prompts: **[1] Pre-built binary** (recommended, no Go needed) or **[2] Build from source**.

### Pre-built binaries

| Platform | Asset |
|---|---|
| Linux x86_64 | \`cmdctx-linux-amd64\` |
| Linux ARM64 | \`cmdctx-linux-arm64\` |
| macOS Intel | \`cmdctx-darwin-amd64\` |
| macOS Apple Silicon | \`cmdctx-darwin-arm64\` |

### Highlights
- Natural language → safe command generation
- Rule-based parser (works fully offline)
- AI providers: Ollama, OpenAI, Anthropic
- Hardcoded safety policy, never runs blocked commands
- Machine + project context scanning
- SQLite history with full redaction
- Bubble Tea TUI (6 screens) + full Cobra CLI"

# ── create release ────────────────────────────────────────────────────────────
echo "=== Creating GitHub release ${TAG} ==="

PAYLOAD=$(jq -n \
  --arg tag  "$TAG" \
  --arg name "$TAG" \
  --arg body "$BODY" \
  '{tag_name:$tag, name:$name, body:$body, draft:false, prerelease:false}')

RELEASE_JSON=$(curl -sf -X POST \
  -H "Authorization: token ${GH_TOKEN}" \
  -H "Accept: application/vnd.github+json" \
  -H "Content-Type: application/json" \
  -d "$PAYLOAD" \
  "${API}/releases")

RELEASE_URL=$(echo "$RELEASE_JSON" | jq -r '.html_url')
UPLOAD_URL=$(echo "$RELEASE_JSON"  | jq -r '.upload_url' | sed 's/{?name,label}//')

echo "  Created: ${RELEASE_URL}"
echo "  Upload URL: ${UPLOAD_URL}"

# ── upload binaries ───────────────────────────────────────────────────────────
echo ""
echo "=== Uploading binaries ==="

for PLATFORM in linux-amd64 linux-arm64 darwin-amd64 darwin-arm64; do
  SRC="${DIST}/cmdctx-${TAG}-${PLATFORM}"
  ASSET="cmdctx-${PLATFORM}"

  if [ ! -f "$SRC" ]; then
    echo "  SKIP  ${ASSET}  (file not found: ${SRC})"
    continue
  fi

  SIZE=$(du -h "$SRC" | cut -f1)
  printf "  %-28s  %s  … " "$ASSET" "$SIZE"

  RESULT=$(curl -sf -X POST \
    -H "Authorization: token ${GH_TOKEN}" \
    -H "Accept: application/vnd.github+json" \
    -H "Content-Type: application/octet-stream" \
    --data-binary "@${SRC}" \
    "${UPLOAD_URL}?name=${ASSET}")

  STATE=$(echo "$RESULT" | jq -r '.state // "error"')
  echo "✓  (${STATE})"
done

# ── verify ────────────────────────────────────────────────────────────────────
echo ""
echo "=== Release summary ==="
curl -sf \
  -H "Authorization: token ${GH_TOKEN}" \
  -H "Accept: application/vnd.github+json" \
  "${API}/releases/latest" | \
  jq '{tag:.tag_name, url:.html_url, assets:[.assets[]|{name:.name, mb:(.size/1048576|floor)}]}'
