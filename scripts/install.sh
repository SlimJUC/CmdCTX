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
#
#  Flags (when running the script directly, not via pipe):
#    --binary       Install pre-built binary from GitHub releases (no Go needed)
#    --source       Build from source (requires Go or auto-installs it)
#    --global       Force install to /usr/local/bin  (requires sudo / root)
#    --no-path      Skip shell profile PATH patching
#    --uninstall    Remove the installed binary (leaves ~/.cmdctx data intact)
# =============================================================================

set -euo pipefail

# ── constants ─────────────────────────────────────────────────────────────────
readonly BINARY="cmdctx"
readonly RELEASES_BASE="https://github.com/SlimJUC/CmdCTX/releases"
readonly DEFAULT_REPO_URL="https://github.com/SlimJUC/CmdCTX.git"
readonly DEFAULT_REPO_BRANCH="main"
# Minimum Go version the project requires (from go.mod).
# The installer falls back to the latest stable if this version is not yet
# published on dl.google.com.
readonly REQUIRED_GO_MAJOR=1
readonly REQUIRED_GO_MINOR=26
readonly REQUIRED_GO_PATCH=2
readonly FALLBACK_GO_VERSION="1.24.2"

REPO_URL="${REPO_URL:-$DEFAULT_REPO_URL}"
REPO_BRANCH="${REPO_BRANCH:-$DEFAULT_REPO_BRANCH}"

# ── runtime state ─────────────────────────────────────────────────────────────
INSTALL_MODE=""      # "binary" | "source"  — resolved by choose_install_mode()
UNINSTALL=false
FORCE_GLOBAL=false
PATCH_PATH=true
TMPDIR_CREATED=""

# ── colours ───────────────────────────────────────────────────────────────────
if [ -t 1 ]; then
  C_RESET="\033[0m"
  C_BOLD="\033[1m"
  C_GREEN="\033[0;32m"
  C_YELLOW="\033[0;33m"
  C_CYAN="\033[0;36m"
  C_RED="\033[0;31m"
  C_DIM="\033[2m"
else
  C_RESET=""; C_BOLD=""; C_GREEN=""; C_YELLOW=""; C_CYAN=""; C_RED=""; C_DIM=""
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
    --binary)    INSTALL_MODE="binary" ;;
    --source)    INSTALL_MODE="source" ;;
    --uninstall) UNINSTALL=true ;;
    --global)    FORCE_GLOBAL=true ;;
    --no-path)   PATCH_PATH=false ;;
    --help|-h)
      sed -n '3,16p' "$0" | sed 's/^# *//'
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
    *)      die "Unsupported operating system: $os  (cmdctx supports Linux and macOS)" ;;
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

# ── version comparison ────────────────────────────────────────────────────────
# Returns 0 (true) if $1 >= $2  (both "MAJOR.MINOR.PATCH")
version_gte() {
  local IFS='.'
  # shellcheck disable=SC2206
  local a=($1) b=($2)
  local i
  for i in 0 1 2; do
    local av=${a[$i]:-0} bv=${b[$i]:-0}
    if   (( av > bv )); then return 0
    elif (( av < bv )); then return 1
    fi
  done
  return 0
}

# ── install mode selection ────────────────────────────────────────────────────
choose_install_mode() {
  # Already set via --binary / --source flag.
  if [ -n "$INSTALL_MODE" ]; then
    return
  fi

  # Non-interactive: no /dev/tty (pure pipe, CI, etc.) → pre-built is fastest
  # and requires no Go toolchain.
  if [ ! -e /dev/tty ]; then
    INSTALL_MODE="binary"
    info "Non-interactive environment — defaulting to pre-built binary"
    return
  fi

  header "Installation method"
  printf "\n"
  printf "  How would you like to install ${C_BOLD}${BINARY}${C_RESET}?\n\n"
  printf "    ${C_BOLD}[1]${C_RESET}  Pre-built binary  "
  printf "${C_GREEN}(recommended — fast, no Go required)${C_RESET}\n"
  printf "    ${C_BOLD}[2]${C_RESET}  Build from source "
  printf "${C_DIM}(Go ${REQUIRED_GO_MAJOR}.${REQUIRED_GO_MINOR}.${REQUIRED_GO_PATCH}+ required — auto-installed if missing)${C_RESET}\n"
  printf "\n"
  printf "  Enter choice [1]: "

  local choice
  read -r choice </dev/tty
  choice="${choice:-1}"

  case "$choice" in
    1 | "") INSTALL_MODE="binary" ;;
    2)      INSTALL_MODE="source" ;;
    *)
      warn "Unrecognised choice '${choice}' — using pre-built binary"
      INSTALL_MODE="binary"
      ;;
  esac
}

# ── pre-built binary install ──────────────────────────────────────────────────
install_prebuilt() {
  header "Downloading pre-built binary"

  # Asset names on GitHub releases (versionless — always points to latest).
  local asset_name="cmdctx-${OS}-${ARCH}"
  local download_url="${RELEASES_BASE}/latest/download/${asset_name}"

  info "Downloading ${asset_name} from GitHub releases…"
  info "URL: ${download_url}"

  local tmp_bin
  tmp_bin="$(mktemp)"

  if ! curl -fsSL --progress-bar "$download_url" -o "$tmp_bin"; then
    rm -f "$tmp_bin"
    die "Download failed.
       URL:  ${download_url}
       Hint: If this is a fresh release, try --source to build from the repo."
  fi

  chmod +x "$tmp_bin"

  # Smoke-test: make sure the binary actually executes on this platform.
  if ! "$tmp_bin" --help >/dev/null 2>&1; then
    rm -f "$tmp_bin"
    die "Downloaded binary failed to run.
       This usually means an architecture mismatch.
       Re-run with --source to build a native binary."
  fi

  resolve_install_dir
  mkdir -p "$INSTALL_DIR" 2>/dev/null || sudo mkdir -p "$INSTALL_DIR" \
    || die "Cannot create install directory: ${INSTALL_DIR}"

  local dst="${INSTALL_DIR}/${BINARY}"

  if [ -w "$INSTALL_DIR" ]; then
    cp "$tmp_bin" "$dst"
    chmod 755 "$dst"
  else
    info "${INSTALL_DIR} requires elevated privileges — using sudo"
    sudo cp "$tmp_bin" "$dst"
    sudo chmod 755 "$dst"
  fi

  rm -f "$tmp_bin"
  success "Installed: ${dst}"
}

# ── Go toolchain ──────────────────────────────────────────────────────────────
ensure_go() {
  header "Checking Go toolchain"

  local required="${REQUIRED_GO_MAJOR}.${REQUIRED_GO_MINOR}.${REQUIRED_GO_PATCH}"
  local install_ver="$required"
  local go_dl_url="https://dl.google.com/go/go${install_ver}.${OS}-${ARCH}.tar.gz"

  # Verify the exact version tarball exists; fall back to latest stable if not.
  if ! curl -fsSL --head "$go_dl_url" >/dev/null 2>&1; then
    warn "Go ${required} not yet published on dl.google.com — falling back to ${FALLBACK_GO_VERSION}"
    install_ver="$FALLBACK_GO_VERSION"
    go_dl_url="https://dl.google.com/go/go${install_ver}.${OS}-${ARCH}.tar.gz"
  fi

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

# ── build from source ─────────────────────────────────────────────────────────
install_from_source() {
  require git
  require make
  ensure_go

  header "Cloning repository"

  TMPDIR_CREATED="$(mktemp -d)"
  local src_dir="${TMPDIR_CREATED}/cmdctx"

  info "Cloning ${REPO_URL} (branch: ${REPO_BRANCH})…"
  git clone --depth=1 --branch "$REPO_BRANCH" "$REPO_URL" "$src_dir" \
    || die "Failed to clone repository: ${REPO_URL}"

  success "Repository cloned"

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

  header "Installing binary"

  resolve_install_dir
  mkdir -p "$INSTALL_DIR" 2>/dev/null || sudo mkdir -p "$INSTALL_DIR" \
    || die "Cannot create install directory: ${INSTALL_DIR}"

  local dst="${INSTALL_DIR}/${BINARY}"

  if [ -w "$INSTALL_DIR" ]; then
    cp "${TMPDIR_CREATED}/${BINARY}" "$dst"
    chmod 755 "$dst"
  else
    info "${INSTALL_DIR} requires elevated privileges — using sudo"
    sudo cp "${TMPDIR_CREATED}/${BINARY}" "$dst"
    sudo chmod 755 "$dst"
  fi

  success "Installed: ${dst}"
}

# ── shell profile patching ────────────────────────────────────────────────────
patch_path() {
  local dir="$1"

  case ":$PATH:" in
    *":$dir:"*) return ;;
  esac

  if ! $PATCH_PATH; then
    warn "Skipping PATH patch (--no-path).  Add manually:  export PATH=\"${dir}:\$PATH\""
    return
  fi

  local profile="" shell_name
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

# ── next steps ────────────────────────────────────────────────────────────────
print_done() {
  local installed_ver
  installed_ver="$("${INSTALL_DIR}/${BINARY}" --version 2>/dev/null | head -1 || echo "")"

  header "Done 🎉"
  printf "\n"
  if [ -n "$installed_ver" ]; then
    printf "  ${C_BOLD}%s${C_RESET} is ready.\n\n" "$installed_ver"
  else
    printf "  ${C_BOLD}${BINARY}${C_RESET} is ready.\n\n"
  fi
  printf "  Next steps:\n"
  printf "    1.  Open a new terminal  (or: source your shell profile)\n"
  printf "    2.  ${C_CYAN}%-22s${C_RESET}  # scan machine + project context\n" "cmdctx init"
  printf "    3.  ${C_CYAN}%-22s${C_RESET}  # verify setup\n"                    "cmdctx doctor"
  printf "    4.  ${C_CYAN}%-22s${C_RESET}  # configure an AI provider\n"         "cmdctx providers"
  printf "\n"
  printf "  Docs: https://github.com/SlimJUC/CmdCTX\n\n"
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

  require curl
  detect_platform

  choose_install_mode

  printf "\n  Mode: ${C_BOLD}%s${C_RESET}\n" \
    "$([ "$INSTALL_MODE" = "binary" ] && echo "pre-built binary" || echo "build from source")"

  if [ "$INSTALL_MODE" = "binary" ]; then
    install_prebuilt
  else
    install_from_source
  fi

  patch_path "$INSTALL_DIR"
  print_done
}

# ── entry point ───────────────────────────────────────────────────────────────
printf "\n${C_BOLD}  cmdctx installer${C_RESET}\n"
printf "  ─────────────────────────────────────────────\n"

if $UNINSTALL; then
  run_uninstall
else
  run_install
fi
