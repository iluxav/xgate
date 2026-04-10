#!/usr/bin/env bash
# xgate installer — downloads the latest release binary, sets up a service.
# Run as: curl -fsSL https://raw.githubusercontent.com/iluxav/xgate/main/install.sh | sudo bash
set -euo pipefail

REPO="iluxav/xgate"

BIN_PATH="/usr/local/bin/xgate"
CONFIG_DIR="/etc/xgate"
CONFIG_PATH="$CONFIG_DIR/config.yaml"
SERVICE_NAME="xgate"

require_root() {
  if [ "$(id -u)" -ne 0 ]; then
    echo "xgate installer requires root; re-running with sudo…" >&2
    exec sudo -E bash "$0" "$@"
  fi
}

detect_os() {
  case "$(uname -s)" in
    Linux)  echo "linux" ;;
    Darwin) echo "darwin" ;;
    *)      echo "unsupported OS: $(uname -s)" >&2; exit 1 ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64)  echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *)             echo "unsupported arch: $(uname -m)" >&2; exit 1 ;;
  esac
}

resolve_version() {
  if [ -n "${XGATE_VERSION:-}" ]; then
    echo "$XGATE_VERSION"
    return
  fi
  local url="https://api.github.com/repos/$REPO/releases/latest"
  local tag
  tag=$(curl -fsSL "$url" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)
  if [ -z "$tag" ]; then
    echo "could not resolve latest release tag from $url" >&2
    exit 1
  fi
  echo "$tag"
}

download_and_extract() {
  local os="$1" arch="$2" version="$3" workdir="$4"
  local tarball="xgate_${os}_${arch}.tar.gz"
  local base="https://github.com/$REPO/releases/download/$version"

  echo "downloading $base/$tarball"
  curl -fsSL -o "$workdir/$tarball" "$base/$tarball"

  echo "downloading $base/sha256sums.txt"
  curl -fsSL -o "$workdir/sha256sums.txt" "$base/sha256sums.txt"

  (cd "$workdir" && grep " $tarball\$" sha256sums.txt | shasum -a 256 -c -) \
    || (cd "$workdir" && grep " $tarball\$" sha256sums.txt | sha256sum -c -)

  tar -xzf "$workdir/$tarball" -C "$workdir"
  if [ ! -f "$workdir/xgate" ]; then
    echo "tarball did not contain an xgate binary" >&2
    exit 1
  fi
}

install_binary() {
  local workdir="$1"
  install -m 0755 "$workdir/xgate" "$BIN_PATH"
  echo "installed $BIN_PATH"
}

ensure_config_dir() {
  mkdir -p "$CONFIG_DIR"
  chmod 0755 "$CONFIG_DIR"
  if [ ! -f "$CONFIG_PATH" ]; then
    cat > "$CONFIG_PATH" <<'EOF'
listen: ":80"
manage_hosts: true
routes: []
EOF
    chmod 0644 "$CONFIG_PATH"
    echo "seeded $CONFIG_PATH"
  else
    echo "kept existing $CONFIG_PATH"
  fi
}

install_linux_service() {
  cat > /etc/systemd/system/${SERVICE_NAME}.service <<EOF
[Unit]
Description=xgate local reverse proxy
After=network.target

[Service]
Type=simple
ExecStart=$BIN_PATH serve
Restart=on-failure
RestartSec=2
User=root
AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable --now "$SERVICE_NAME"
  echo "systemd: enabled and started $SERVICE_NAME"
}

install_macos_service() {
  local plist="/Library/LaunchDaemons/com.xgate.daemon.plist"
  cat > "$plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.xgate.daemon</string>
  <key>ProgramArguments</key>
  <array>
    <string>$BIN_PATH</string>
    <string>serve</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>UserName</key><string>root</string>
  <key>StandardOutPath</key><string>/var/log/xgate.log</string>
  <key>StandardErrorPath</key><string>/var/log/xgate.log</string>
</dict>
</plist>
EOF
  chmod 0644 "$plist"
  launchctl unload "$plist" 2>/dev/null || true
  launchctl load "$plist"
  echo "launchd: loaded $plist"
}

verify_running() {
  local socket="/var/run/xgate.sock"
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    if [ -S "$socket" ]; then
      echo "daemon is up"
      return 0
    fi
    sleep 0.3
  done
  echo "WARNING: daemon did not open $socket in time — check logs" >&2
  return 1
}

main() {
  require_root "$@"
  local os arch version workdir
  os="$(detect_os)"
  arch="$(detect_arch)"
  version="$(resolve_version)"
  workdir="$(mktemp -d)"
  trap 'rm -rf "$workdir"' EXIT

  echo "xgate $version for $os/$arch"
  download_and_extract "$os" "$arch" "$version" "$workdir"
  install_binary "$workdir"
  ensure_config_dir

  case "$os" in
    linux)  install_linux_service ;;
    darwin) install_macos_service ;;
  esac

  verify_running || true

  cat <<EOF

xgate installed.
  binary:  $BIN_PATH
  config:  $CONFIG_PATH
  logs:    $( [ "$os" = "linux" ] && echo "journalctl -u xgate -f" || echo "tail -f /var/log/xgate.log" )

try:
  sudo xgate add hello.localhost http://localhost:3000
  sudo xgate ls
EOF
}

main "$@"
