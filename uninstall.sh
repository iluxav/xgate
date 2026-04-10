#!/usr/bin/env bash
# xgate uninstaller — stops the service and removes the binary.
# Leaves /etc/xgate/ in place so routes are not silently lost.
set -euo pipefail

BIN_PATH="/usr/local/bin/xgate"
CONFIG_DIR="/etc/xgate"
SERVICE_NAME="xgate"

require_root() {
  if [ "$(id -u)" -ne 0 ]; then
    echo "xgate uninstaller requires root; re-running with sudo…" >&2
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

uninstall_linux() {
  if systemctl list-unit-files | grep -q "^${SERVICE_NAME}.service"; then
    systemctl disable --now "$SERVICE_NAME" || true
  fi
  rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
  systemctl daemon-reload || true
  echo "systemd: disabled $SERVICE_NAME"
}

uninstall_macos() {
  local plist="/Library/LaunchDaemons/com.xgate.daemon.plist"
  if [ -f "$plist" ]; then
    launchctl unload "$plist" 2>/dev/null || true
    rm -f "$plist"
  fi
  echo "launchd: unloaded com.xgate.daemon"
}

main() {
  require_root "$@"
  case "$(detect_os)" in
    linux)  uninstall_linux ;;
    darwin) uninstall_macos ;;
  esac

  if [ -f "$BIN_PATH" ]; then
    rm -f "$BIN_PATH"
    echo "removed $BIN_PATH"
  fi

  if [ -d "$CONFIG_DIR" ]; then
    echo
    echo "kept $CONFIG_DIR (your routes). Remove manually with:"
    echo "  sudo rm -rf $CONFIG_DIR"
  fi
}

main "$@"
