# xgate

A tiny local reverse proxy for developers. Runs as a system service on port 80, manages `/etc/hosts` entries for you, and ships with a CLI that adds/removes routes live — no restart needed.

```
sudo xgate add api.localhost http://localhost:8081
sudo xgate add app.localhost http://localhost:5173
sudo xgate ls
```

That's it. `http://api.localhost` now proxies to `localhost:8081`.

## Features

- **Host-based routing** — map `app.localhost`, `api.localhost`, etc. to different upstream ports.
- **Wildcard hosts** — `*.app.localhost` matches any subdomain.
- **Live reload** — `xgate add`/`rm` mutate the running daemon; no restart, no dropped connections.
- **`/etc/hosts` management** — entries are added/removed automatically.
- **One-liner install** — on Linux (systemd) and macOS (launchd).
- **Config-file driven** — routes persist in `/etc/xgate/config.yaml`.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/iluxav/xgate/main/install.sh | sudo bash
```

The installer:

1. Detects your OS (Linux or macOS) and arch (amd64 or arm64).
2. Downloads the latest release binary from GitHub.
3. Verifies the SHA-256 checksum.
4. Installs the binary to `/usr/local/bin/xgate`.
5. Creates `/etc/xgate/config.yaml` with a default config (if missing).
6. Writes a systemd unit (Linux) or launchd plist (macOS).
7. Enables and starts the service.

After install, `xgate` is running on port 80 with an empty route table.

### Pin a specific version

```bash
curl -fsSL https://raw.githubusercontent.com/iluxav/xgate/main/install.sh | sudo XGATE_VERSION=v0.1.0 bash
```

### Uninstall

```bash
curl -fsSL https://raw.githubusercontent.com/iluxav/xgate/main/uninstall.sh | sudo bash
```

This stops and removes the service and the binary. Your `/etc/xgate/` directory is left in place so your routes aren't silently destroyed — remove it manually with `sudo rm -rf /etc/xgate` if you're done for good.

## Build from source

Requires Go 1.25+.

```bash
git clone https://github.com/iluxav/xgate.git
cd xgate
make build
sudo ./xgate serve
```

## Usage

All mutating CLI commands require `sudo` (the admin socket is root-owned, mode `0600`).

### Add a route

```bash
sudo xgate add gateway.localhost http://localhost:8081
```

Output:
```
added gateway.localhost -> http://localhost:8081 (1 routes)
```

### Wildcard subdomains

```bash
sudo xgate add '*.app.localhost' http://localhost:5173
```

Now `http://anything.app.localhost` proxies to `localhost:5173`. Exact matches always beat wildcard matches, so you can mix them freely.

On macOS, `*.localhost` resolves to `127.0.0.1` automatically. On Linux, you need either individual `/etc/hosts` entries (xgate manages these for exact hosts) or a local resolver like `dnsmasq` configured for `*.localhost`.

### List routes

```bash
sudo xgate ls
```

Output:
```
HOST                 TARGET
gateway.localhost    http://localhost:8081
*.app.localhost      http://localhost:5173
```

If the daemon isn't running, `ls` falls back to reading `/etc/xgate/config.yaml` directly — this works without `sudo` because the config file is `0644`.

### Remove a route

```bash
sudo xgate rm gateway.localhost
```

### Reload config from disk

If you hand-edit `/etc/xgate/config.yaml`, tell the daemon to re-read it:

```bash
sudo xgate reload
```

## Configuration

The config file lives at `/etc/xgate/config.yaml`:

```yaml
listen: ":80"
manage_hosts: true
routes:
  - host: gateway.localhost
    target: http://localhost:8081
  - host: "*.api.localhost"
    target: http://localhost:8081
  - host: app.localhost
    target: http://localhost:5173
```

- **`listen`** — address to bind the HTTP server. Default `:80`. Change to `:8080` (or similar) if you don't want to run as root. Changing this requires a service restart: `sudo systemctl restart xgate` / `sudo launchctl unload … && sudo launchctl load …`.
- **`manage_hosts`** — whether xgate should add/remove entries in `/etc/hosts` automatically. Wildcard routes are skipped. Default `true`.
- **`routes`** — list of `host → target` mappings. Prefer managing these via `xgate add`/`rm` rather than editing by hand, because writes from the CLI drop YAML comments on rewrite.

## How it works

xgate is a single Go binary that plays two roles:

- **`xgate serve`** — the daemon. Runs the HTTP reverse proxy on `:80` and an admin Unix socket at `/var/run/xgate.sock` (mode `0600`). This is what the service unit launches.
- **`xgate add|rm|ls|reload`** — the CLI. Dials the admin socket, sends one JSON command, prints the response.

Live reloads work via an `atomic.Pointer[Router]` inside the daemon: when a route is added or removed, the daemon builds a fresh routing table in memory and atomically swaps the pointer. In-flight requests finish on the old table, new requests see the new one — no locks on the hot path.

## Flag and environment overrides

Useful for running a non-root dev daemon against a local config without clobbering your installed instance:

```bash
xgate --config /tmp/dev.yaml --socket /tmp/xgate.sock serve
xgate --config /tmp/dev.yaml --socket /tmp/xgate.sock add test.local http://localhost:3000
```

| Flag         | Env var          | Default                   |
|--------------|------------------|---------------------------|
| `--config`   | `XGATE_CONFIG`   | `/etc/xgate/config.yaml`  |
| `--socket`   | `XGATE_SOCKET`   | `/var/run/xgate.sock`     |

Priority: flag > env var > default.

## Logs

**Linux:**
```bash
journalctl -u xgate -f
```

**macOS:**
```bash
tail -f /var/log/xgate.log
```

## Troubleshooting

**"xgate daemon not running"** — the CLI couldn't reach `/var/run/xgate.sock`. Start the service:

- Linux: `sudo systemctl start xgate`
- macOS: `sudo launchctl load /Library/LaunchDaemons/com.xgate.daemon.plist`

**"bind: permission denied" in logs** — the daemon can't bind to port 80. On Linux the systemd unit grants `CAP_NET_BIND_SERVICE`; if you're running outside the unit, use `sudo` or change `listen` in the config to an unprivileged port.

**"no route for host: …"** — you requested a hostname that doesn't match any route. Check `sudo xgate ls`. Remember that exact matches beat wildcards, and wildcards need a `*.` prefix.

**Routes don't resolve** — the browser might be resolving the hostname directly instead of going through xgate. Check `/etc/hosts` — for exact-match hosts the installer keeps a marker block there. For wildcards on Linux you need a DNS-level solution (dnsmasq).

## License

MIT.
