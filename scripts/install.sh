#!/usr/bin/env bash
# =============================================================================
#  cmdctx — one-line installer
#
#  Usage:
#    curl -fsSL https://raw.githubusercontent.com/SlimJUC/CmdCTX/main/scripts/install.sh | bash
#
#  Environment overrides (set before piping to bash):
#    INSTALL_DIR    Target directory  (default: ~/.local/bin  or  /usr/local/bin when root)
#    REPO_URL       Git clone URL     (default: https://github.com/SlimJUC/CmdCTX.git)
#    REPO_BRANCH    Branch/tag        (default: main)
#    GO_VERSION     Go toolchain ver  (default: 1.24.2 — latest stable as fallback)
#
#  Flags (when running the script directly, not via pipe):
#    --uninstall    Remove the installed binary (leaves ~/.cmdctx data intact)
#    --global       Force install to /usr/local/bin  (requires sudo / root)
#    --no-path      Skip shell profile PATH patching
# =============================================================================

set -euo pipefail

# ── constants ─────────────────────────────────────────────────────────────────
readonly BINARY="cmdctx"
readonly MODULE="github.com/slim/cmdctx"
readonly DEFAULT_REPO_URL="https://github.com/SlimJUC/CmdCTX.git"
readonly DEFAULT_REPO_BRANCH="main"
# Minimum Go version the project requires (from go.mod: go 1.26.2).
# We also carry a known-stable fallback in case the project's required
# version has not been officially released yet on dl.google.com.
readonly REQUIRED_GO_MAJOR=1
readonly REQUIRED_GO_MINOR=26
readonly REQUIRED_GO_PATCH=2
readonly FALLBACK_GO_VERSION="1.24.2"   # latest stable — update as needed

REPO_URL="${REPO_URL:-$DEFAULT_REPO_URL}"
REPO_BRANCH="${REPO_BRANCH:-$DEFAULT_REPO_BRANCH}"

# ── runtime state ─────────────────────────────────────────────────────────────
UNINSTALL=false
FORCE_GLOBAL=false
PATCH_PATH=true
TMPDIR_CREATED=""

# ── colours ───────────────────────────────────────────────────────────────────
# Only use colours when stdout is a real terminal (not a pipe).
if [ -t 1 ]; then
  C_RESET="\033[0m"
  C_BOLD="\033[1m"
  C_GREEN="\033[0;32m"
  C_YELLOW="\033[0;33m"
  C_CYAN="\033[0;36m"
  C_RED="\033[0;31m"
else
  C_RESET=""; C_BOLD=""; C_GREEN=""; C_YELLOW=""; C_CYAN=""; C_RED=""
fi

info()    { printf "  ${C_CYAN}▶${C_RESET}  %s\n" "$*"; }
success() { printf "  ${C_GREEN}✓${C_RESET}  %s\n" "$*"; }
warn()    { printf "  ${C_YELLOW}⚠${C_RESET}  %s\n" "$*"; }
die()     { printf "  ${C_RED}✗${C_RESET}  %s\n" "$*" >&2; exit 1; }
header()  { printf "\n${C_BOLD}%s${C_RESET}\n" "$*"; }

# ── cleanup trap ──────────────────────────────────────────────────────────────
cleanup() {
  if [ -n "$TMPDIR_CREATED" ] && [ -d "$TMPDIR_CREATED" ]; then
    rm -rf "$TMPDIR_CREATED"
  fi
}
trap cleanup EXIT INT TERM

# ── argument parsing ──────────────────────────────────────────────────────────
for arg in "$@"; do
  case "$arg" in
    --uninstall) UNINSTALL=true ;;
    --global)    FORCE_GLOBAL=true ;;
    --no-path)   PATCH_PATH=false ;;
    --help|-h)
      sed -n '3,14p' "$0" | sed 's/^# *//'
      exit 0
      ;;
    *) die "Unknown argument: $arg  (run with --help for usage)" ;;
  esac
done

# ── platform detection ────────────────────────────────────────────────────────
detect_platform() {
  local os arch

  os="$(uname -s)"
  arch="$(uname -m)"

  case "$os" in
    Linux)  OS="linux" ;;
    Darwin) OS="darwin" ;;
    *)      die "Unsupported operating system: $os  (cmdctx requires Linux or macOS)" ;;
  esac

  case "$arch" in
    x86_64)          ARCH="amd64" ;;
    aarch64 | arm64) ARCH="arm64" ;;
    armv6l | armv7l) ARCH="armv6l" ;;
    *)               die "Unsupported CPU architecture: $arch" ;;
  esac
}

# ── install directory ─────────────────────────────────────────────────────────
resolve_install_dir() {
  if [ -n "${INSTALL_DIR:-}" ]; then
    return
  fi
  if $FORCE_GLOBAL || [ "$(id -u)" -eq 0 ]; then
    INSTALL_DIR="/usr/local/bin"
  else
    INSTALL_DIR="$HOME/.local/bin"
  fi
}

# ── require a command ─────────────────────────────────────────────────────────
require() {
  command -v "$1" >/dev/null 2>&1 || die "Required tool not found: '$1'. Please install it and retry."
}

# ── version comparison helpers ────────────────────────────────────────────────
# Returns 0 if $1 >= $2 (both are "MAJOR.MINOR.PATCH" strings)
version_gte() {
  local IFS='.'
  # shellcheck disable=SC2206
  local a=($1) b=($2)
  local i
  for i in 0 1 2; do
    local av=${a[$i]:-0}
    local bv=${b[$i]:-0}
    if   (( av > bv )); then return 0
    elif (( av < bv )); then return 1
    fi
  done
  return 0   # equal → also satisfies >=
}

# ── Go installation / detection ───────────────────────────────────────────────
ensure_go() {
  header "Checking Go toolchain"

  local required="${REQUIRED_GO_MAJOR}.${REQUIRED_GO_MINOR}.${REQUIRED_GO_PATCH}"

  # Resolve which Go to install if the required version isn't available yet.
  # dl.google.com only carries officially released versions; if the project's
  # go.mod requires a future version we transparently fall back to the latest
  # stable and let `go build` handle any actual compatibility issues.
  local install_ver="$required"
  local go_dl_url="https://dl.google.com/go/go${install_ver}.${OS}-${ARCH}.tar.gz"

  # Quick head request to verify the tarball exists; fall back if not.
  if ! curl -fsSL --head "$go_dl_url" >/dev/null 2>&1; then
    warn "Go ${required} not yet published on dl.google.com — falling back to ${FALLBACK_GO_VERSION}"
    install_ver="$FALLBACK_GO_VERSION"
    go_dl_url="https://dl.google.com/go/go${install_ver}.${OS}-${ARCH}.tar.gz"
  fi

  # Check if a suitable Go is already on PATH.
  if command -v go >/dev/null 2>&1; then
    local current_ver
    current_ver="$(go version | grep -oE '[0-9]+\.[0-9]+(\.[0-9]+)?' | head -1)"
    if version_gte "$current_ver" "$required"; then
      success "Go ${current_ver} already installed (need >= ${required})"
      export GOPATH="${GOPATH:-$HOME/go}"
      export PATH="$GOPATH/bin:$PATH"
      return
    fi
    warn "Go ${current_ver} found but need >= ${required} — installing ${install_ver}"
  else
    info "Go not found — installing ${install_ver}"
  fi

  # Download and install Go to /usr/local/go
  local tarball
  tarball="$(mktemp --suffix=.tar.gz)"
  info "Downloading Go ${install_ver} (${OS}/${ARCH})…"
  curl -fsSL --progress-bar "$go_dl_url" -o "$tarball" \
    || die "Failed to download Go from: $go_dl_url"

  info "Installing Go to /usr/local/go (may require sudo)…"
  if [ "$(id -u)" -eq 0 ]; then
    rm -rf /usr/local/go
    tar -C /usr/local -xzf "$tarball"
  else
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf "$tarball"
  fi
  rm -f "$tarball"

  export PATH="/usr/local/go/bin:$PATH"
  export GOPATH="${GOPATH:-$HOME/go}"
  export PATH="$GOPATH/bin:$PATH"

  success "Go $(go version | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1) installed"
}

# ── shell profile patching ────────────────────────────────────────────────────
# Appends PATH export to the user's shell profile (idempotent).
patch_path() {
  local dir="$1"

  # Already in PATH — nothing to do.
  case ":$PATH:" in
    *":$dir:"*) return ;;
  esac

  if ! $PATCH_PATH; then
    warn "Skipping PATH patch (--no-path).  Add manually:  export PATH=\"${dir}:\$PATH\""
    return
  fi

  # Detect the current interactive shell's profile file.
  local profile=""
  local shell_name
  shell_name="$(basename "${SHELL:-/bin/sh}")"

  case "$shell_name" in
    zsh)  profile="${ZDOTDIR:-$HOME}/.zshrc" ;;
    bash) profile="$HOME/.bashrc" ;;
    fish) profile="$HOME/.config/fish/config.fish" ;;
    *)    profile="$HOME/.profile" ;;
  esac

  local export_line
  if [ "$shell_name" = "fish" ]; then
    export_line="fish_add_path \"${dir}\""
  else
    export_line="export PATH=\"${dir}:\$PATH\""
  fi

  # Idempotency: don't add the line if it's already there.
  if grep -qF "$dir" "$profile" 2>/dev/null; then
    return
  fi

  {
    printf '\n# cmdctx — added by installer\n'
    printf '%s\n' "$export_line"
  } >> "$profile"

  warn "${dir} added to ${profile}"
  warn "Run:  source ${profile}   (or open a new terminal) to update your PATH"
}

# ── uninstall ─────────────────────────────────────────────────────────────────
run_uninstall() {
  resolve_install_dir
  local target="${INSTALL_DIR}/${BINARY}"

  header "Uninstalling ${BINARY}"

  if [ ! -f "$target" ]; then
    warn "Binary not found at ${target} — nothing to remove"
    exit 0
  fi

  if [ -w "$target" ] || [ -w "$INSTALL_DIR" ]; then
    rm -f "$target"
  else
    sudo rm -f "$target"
  fi

  success "Removed ${target}"
  info "App data at ~/.cmdctx was NOT removed."
  info "To remove all data run:  rm -rf ~/.cmdctx"
}

# ── main install flow ─────────────────────────────────────────────────────────
run_install() {
  header "Installing ${BINARY}"

  # ── 1. prerequisites ────────────────────────────────────────────────────────
  require curl
  require git
  require make

  detect_platform
  resolve_install_dir
  ensure_go

  # ── 2. clone repository ─────────────────────────────────────────────────────
  header "Cloning repository"

  TMPDIR_CREATED="$(mktemp -d)"
  local src_dir="${TMPDIR_CREATED}/cmdctx"

  info "Cloning ${REPO_URL} (branch: ${REPO_BRANCH})…"
  git clone --depth=1 --branch "$REPO_BRANCH" "$REPO_URL" "$src_dir" \
    || die "Failed to clone repository: ${REPO_URL}"

  success "Repository cloned to ${src_dir}"

  # ── 3. build ────────────────────────────────────────────────────────────────
  header "Building"

  local version commit build_date
  version="$(git -C "$src_dir" describe --tags --always --dirty 2>/dev/null || echo "dev")"
  commit="$(git -C "$src_dir" rev-parse --short HEAD 2>/dev/null || echo "unknown")"
  build_date="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  local ldflags="-X main.version=${version} -X main.commit=${commit} -X main.buildDate=${build_date} -s -w"

  info "Building ${BINARY} ${version}…"
  (
    cd "$src_dir"
    go build -ldflags "$ldflags" -o "${TMPDIR_CREATED}/${BINARY}" ./cmd/cmdctx
  ) || die "Build failed — check the output above for errors"

  success "Build successful  (${BINARY} ${version})"

  # ── 4. install binary ───────────────────────────────────────────────────────
  header "Installing binary"

  mkdir -p "$INSTALL_DIR" 2>/dev/null || {
    sudo mkdir -p "$INSTALL_DIR" || die "Cannot create install directory: ${INSTALL_DIR}"
  }

  local src_binary="${TMPDIR_CREATED}/${BINARY}"
  local dst_binary="${INSTALL_DIR}/${BINARY}"

  if [ -w "$INSTALL_DIR" ]; then
    cp "$src_binary" "$dst_binary"
    chmod 755 "$dst_binary"
  else
    info "${INSTALL_DIR} requires elevated privileges — using sudo"
    sudo cp "$src_binary" "$dst_binary"
    sudo chmod 755 "$dst_binary"
  fi

  success "Installed: ${dst_binary}"

  # ── 5. PATH ─────────────────────────────────────────────────────────────────
  patch_path "$INSTALL_DIR"

  # ── 6. done — next steps ────────────────────────────────────────────────────
  header "Done 🎉"
  printf "\n"
  printf "  ${C_BOLD}${BINARY} ${version}${C_RESET} is ready.\n\n"
  printf "  Next steps:\n"
  printf "    1.  Open a new terminal  (or: source your shell profile)\n"
  printf "    2.  ${C_CYAN}%s${C_RESET}          # scan machine + project context\n" "cmdctx init"
  printf "    3.  ${C_CYAN}%s${C_RESET}        # verify setup\n"                      "cmdctx doctor"
  printf "    4.  ${C_CYAN}%s${C_RESET}    # configure an AI provider\n"               "cmdctx providers"
  printf "\n"
  printf "  Docs: https://github.com/SlimJUC/CmdCTX\n\n"
}

# ── entry point ───────────────────────────────────────────────────────────────
printf "\n${C_BOLD}  cmdctx installer${C_RESET}\n"
printf "  ─────────────────────────────────────────────\n"

if $UNINSTALL; then
  run_uninstall
else
  run_install
fi
