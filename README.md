# fm350-monitor

Lightweight **Linux** Go daemon + embedded WebUI for the **Fibocom FM350-GL** 5G USB modem (`0e8d:7127` or `0e8d:7126`).

It keeps USB power stable (autosuspend / `power/control`), talks **AT** over serial, exposes a live dashboard (SSE), can bring up a **data path** when the modem presents RNDIS (network iface) or MBIM (`/dev/cdc-wdm*`), and can share that LTE uplink as a **WiFi hotspot** (hostapd + NAT).

| | |
|---|---|
| Binary entry | `cmd/app` → `go build -o fm350-manager ./cmd/app` |
| WebUI default | `http://0.0.0.0:8080` (override with `-bind` / `-port`) |
| Target OS | Linux (sysfs + USB serial) |
| Go | 1.22+ (module currently `go 1.25`) |

---

## Features (summary)

- **USB watchdog** — disable global USB autosuspend; force `power/control=on`; optional `USBDEVFS_RESET`
- **AT client** — multi-`ttyUSB` probe, reconnect, signal/SIM/APN/RAT, raw AT console
- **Modem inventory UI** — dropdown of devices, AT ports, data interfaces
- **Data bearer**
  - **RNDIS** (common on FM350 USB): `ip link up` + DHCP, then **PDP static** from `AT+CGPADDR` if DHCP assigns nothing
  - **MBIM** (if present): optional `mbimcli`
- **WiFi hotspot** — host WiFi AP (hostapd) + DHCP (dnsmasq) + NAT to LTE/RNDIS uplink
- **Router-style WebUI** — Overview / Cellular / WAN / LAN / Advanced panels
- **Signal history** — in-memory ring buffer + optional JSON file
- **Single binary** — vanilla HTML/CSS/JS via `go:embed` (no Node build)
- **Auto-elevate** — re-exec with `sudo` when not root (`-no-elevate` to skip)

---

## Required libraries & dependencies

### Summary — OS packages (Debian/Ubuntu)

| Feature | Packages (binaries) | Required? |
|---------|---------------------|-----------|
| **Core daemon** (AT, WebUI, USB power) | Linux kernel + root/sudo | **Yes** |
| **WAN / RNDIS data** | `iproute2` (`ip`), `isc-dhcp-client` (`dhclient`) *or* `dhcpcd` | For data connect |
| **WAN / MBIM data** | `libmbim-utils` (`mbimcli`) | Only if `/dev/cdc-wdm*` |
| **LAN / WiFi hotspot** | `hostapd`, `dnsmasq`, `iw`, `nftables` *or* `iptables`, `wireless-regdb` | For software AP + NAT |
| **Build** | `golang-go`, `git` | Build only |
| **Debug** | `usbutils` (`lsusb`), `rfkill` | Optional |

**One-shot install (full cellular router path):**

```bash
# Build
sudo apt-get install -y golang-go git

# Core runtime (WAN RNDIS + system tools)
sudo apt-get install -y iproute2 isc-dhcp-client sudo

# WiFi hotspot (LAN AP + DHCP + NAT)
sudo apt-get install -y hostapd dnsmasq iw nftables wireless-regdb

# Optional: MBIM compositions only
# sudo apt-get install -y libmbim-utils

# Optional debug
# sudo apt-get install -y usbutils
```

Verify:

```bash
command -v ip dhclient hostapd dnsmasq iw nft
# or: command -v dhcpcd iptables
```

The WebUI **LAN → WiFi diagnostics** and `GET /api/hotspot` also report missing tools + an `install_hint`.

### 1. Build-time (Go modules)

| Module | Role | Required? |
|--------|------|-----------|
| **[go.bug.st/serial](https://github.com/bugst/go-serial)** `v1.8.0` | Serial port open/read/write, USB port enumeration | **Yes** |
| **golang.org/x/sys** | Transitive (serial on Linux) | **Yes** (indirect) |

Everything else is **Go standard library** (`net/http`, `os`, `embed`, `html/template`, `testing`, …).

```bash
go mod download   # or: go build -o fm350-manager ./cmd/app
```

### 2. Runtime — OS packages (detail)

| Package / tool | Purpose | When |
|----------------|---------|------|
| **sudo** | Auto-elevate for sysfs/serial/USB/hotspot | Recommended (default) |
| **iproute2** (`ip`) | Link up/down, addresses, routing | RNDIS + hotspot |
| **isc-dhcp-client** or **dhcpcd** | DHCP **client** on RNDIS WAN iface | RNDIS data connect |
| **hostapd** | WiFi **access point** (WPA2) | Hotspot |
| **dnsmasq** | DHCP/DNS **server** for WiFi clients | Hotspot |
| **iw** | Detect AP mode / stations | Hotspot discovery & clients |
| **nftables** (`nft`) or **iptables** | IPv4 forward + MASQUERADE WAN | Hotspot NAT |
| **wireless-regdb** | Regulatory domain for channels | Hotspot (esp. country code) |
| **libmbim-utils** (`mbimcli`) | MBIM connect/disconnect | MBIM-only compositions |
| **usbutils** (`lsusb`) | Manual USB debugging | Optional |

Other distros (approximate):

| Distro | WAN DHCP | Hotspot AP stack | MBIM |
|--------|----------|------------------|------|
| Fedora/RHEL | `dhcp-client` / `dhcpcd` | `hostapd` `dnsmasq` `iw` `nftables` | `libmbim-utils` |
| Arch | `dhclient` / `dhcpcd` | same | `libmbim` |

### 3. Kernel modules / drivers (usually auto-loaded)

| Module / driver | When |
|-----------------|------|
| **option** + **usb_wwan** + **usbserial** | Multi-serial AT (`/dev/ttyUSB*`) |
| **rndis_host** | RNDIS WAN iface (e.g. `enx…`) — **common FM350 mode** |
| **cdc_mbim** | `/dev/cdc-wdm*` if composition is MBIM |
| Host WiFi (`iwlwifi`, etc.) | Software AP on `wlan*` / `wlp*` |

No out-of-tree modem driver is required by this project. Host WiFi must support **AP mode** (`iw phy … info` → `* AP`).

### 4. Device nodes & privileges

| Resource | Access |
|----------|--------|
| `/dev/ttyUSB*` | AT — root **or** group **`dialout`** |
| `/sys/module/usbcore/parameters/autosuspend` | Write — typically **root** |
| `/sys/bus/usb/devices/…/power/control` | Write — typically **root** |
| `/dev/bus/usb/BBB/DDD` | `USBDEVFS_RESET` — typically **root** |
| `/dev/cdc-wdm*` | MBIM — root or udev rules |
| `ip` / DHCP client | RNDIS — typically **root** |
| `hostapd` / `dnsmasq` / `nft` | Hotspot — typically **root** |
| `/run/fm350-manager` | Hotspot conf/logs (systemd `RuntimeDirectory`) |
| `/var/lib/fm350-manager` | History + `hotspot.json` (systemd `StateDirectory`) |

```bash
# Non-root serial only (no hard reset / limited sysfs / no hotspot):
sudo usermod -aG dialout "$USER"   # then re-login
```

Default: if not root, the process **re-execs under `sudo`** (`-no-elevate` to
disable). `make run` / `make dev` run **without root** by passing `-no-elevate`
and local runtime/state dirs. Run `make setup` once to grant serial + sysfs
access to your user (see below). Systemd unit runs as **root**.

### 5. Not required

- Node.js / npm / webpack (UI is embedded vanilla JS)
- ModemManager / NetworkManager (not used; for hotspot, leave wlan **unmanaged** if NM is installed)
- SQLite or other databases (history is memory ± JSON file)

---

## Quick start

```bash
git clone <repo-url> fm350-monitor
cd fm350-monitor

go build -o fm350-manager ./cmd/app

# elevates with sudo automatically if needed
./fm350-manager -bind 127.0.0.1 -port 8080

# or:
go run ./cmd/app -bind 127.0.0.1 -port 8080
```

Open **http://127.0.0.1:8080**

### Useful flags

| Flag | Default | Meaning |
|------|---------|---------|
| `-bind` | `0.0.0.0` | HTTP listen address |
| `-port` | `8080` | HTTP port |
| `-serial` | auto | Force AT port e.g. `/dev/ttyUSB2` |
| `-watch` | `5s` | USB power poll interval |
| `-poll` | `2s` | Background status (AT/USB) poll interval |
| `-history` | `180` | Signal sample ring capacity |
| `-history-file` | _(empty)_ | Persist history JSON |
| `-no-elevate` | false | Do not re-exec with sudo |
| `-token` | _(empty)_ | Shared API token (also `FM350_API_TOKEN`) |

### Development (Make + Air live reload)

```bash
make build          # go build -o fm350-manager ./cmd/app
make run            # build + run on 0.0.0.0:8080 (no sudo)
make run-sudo       # same, as root (AT / WAN DHCP-or-static / hotspot)
make test           # go test ./...
make check          # gofmt check + go vet + tests
make help           # targets + BIND/PORT/SERIAL/ARGS
```

`make run BIND=0.0.0.0 PORT=8080 SERIAL=/dev/ttyUSB2` overrides listen/AT port.
History is written to `.state/history.json`. Extra flags: `ARGS='-poll 5s'`.

`make run` and `make dev` never elevate. They pass `-no-elevate` and point
`RUNTIME_DIRECTORY`/`STATE_DIRECTORY` at gitignored `./.runtime` / `./.state`, so
hotspot conf, logs, history, and `hotspot.json` all stay in the repo and are
user-writable. One-time permissions for the USB modem:

```bash
make setup   # adds $USER to dialout + installs udev rules for sysfs/USB reset/MBIM
             # then re-login (or run: newgrp dialout)
```

After `make setup`, AT monitoring, USB hard reset, and MBIM work without root.
RNDIS DHCP, hostapd/dnsmasq, and NAT still need root — use the systemd unit for
full router functionality. If you do run as root, keep the defaults: the daemon
auto-elevates, `/run/fm350-manager` + `/var/lib/fm350-manager` are used, and
`make run` is just `./fm350-manager -bind 127.0.0.1 -port 8080`.

Live reload with [Air](https://github.com/air-verse/air):

```bash
go install github.com/air-verse/air@latest   # one-time install (adds ~/go/bin to PATH if needed)
make dev            # rebuilds + restarts on .go changes; UI assets are embedded, no restart needed
```

`make dev` runs the app with `-no-elevate` and local runtime/state dirs baked
into `.air.toml`, so the daemon never re-execs under `sudo` mid-reload and never
touches `/run` or `/var/lib`. Air builds into `./tmp/` and writes
`build-errors.log` on failure — all gitignored. Extra flags (e.g.
`-serial /dev/ttyUSB2 -poll 5s`) go in `args_bin` in `.air.toml`.

### Auth (optional)

When `-token` or `FM350_API_TOKEN` is set, every request needs one of:

- `Authorization: Bearer <token>`
- `X-API-Token: <token>`
- `?token=<token>` (required for **SSE** / EventSource)

Open the UI as `http://127.0.0.1:8080/?token=YOUR_TOKEN` (stored in `localStorage`).

### Systemd

See `deploy/fm350-manager.service` (runs as root; **binds 127.0.0.1**; history under `/var/lib/fm350-manager`).

```bash
sudo cp fm350-manager /usr/local/bin/
sudo cp deploy/fm350-manager.service /etc/systemd/system/
# optional: echo 'FM350_API_TOKEN=change-me' | sudo tee /etc/fm350-manager.env
# then uncomment EnvironmentFile= in the unit
sudo systemctl daemon-reload
sudo systemctl enable --now fm350-manager
```

---

## FM350 USB modes (why MBIM may be empty)

Many FM350-GL sticks enumerate as **RNDIS + many `ttyUSB*`**, not MBIM:

| Mode | Kernel | Data path in UI |
|------|--------|-----------------|
| **RNDIS** (typical) | `rndis_host` + `option` | **Data interface** = `enx…` (DHCP) |
| **MBIM** | `cdc_mbim` | `/dev/cdc-wdm*` + `mbimcli` |
| AT-only | serial only | Monitor via AT; no data iface |

Composition is firmware/USB-mode specific. WebUI **USB Composition** can query/set `AT+GTUSBMODE` (stock **40/41 are both RNDIS**, not MBIM). Changing mode re-enumerates USB.

Workflow:

1. **Modem** card → select device + AT port → **Use selected**
2. **APN** → Apply
3. **Data Bearer** → select RNDIS iface (or MBIM if present) → **Connect**
4. Optional: **USB Composition** → switch profile 40/41 if needed

---

## Project layout

```text
cmd/app/                 entrypoint + sudo elevate
internal/pkg/domain/     models + enums
internal/pkg/appdefaults defaults (ports, intervals)
internal/repository/     sysfs, serial, history, mbimcli, RNDIS/ip
internal/usecase/        ModemService workflows
internal/handler/        HTTP + SSE
internal/template/       embedded WebUI
deploy/                  systemd unit
docs/                    architecture + AT guide
```

Details: [`docs/architecture.md`](docs/architecture.md), [`docs/at-command-guide.md`](docs/at-command-guide.md), agent rules: [`AGENTS.md`](AGENTS.md).

---

## API (short)

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/status` | Cached status JSON (`?fresh=1` forces AT poll) |
| GET | `/api/events` | SSE stream of **cache** (shared hub, ~2s; poller owns AT) |
| GET | `/api/modems` | Inventory + selection |
| POST | `/api/modems/select` | Choose modem / AT / data iface |
| POST | `/api/apn` | Set APN |
| POST | `/api/rat` | RAT pref (`5g`/`lte`/`auto`) |
| POST | `/api/raw` | Raw AT |
| POST | `/api/reset` | USB hard reset |
| POST | `/api/data/connect` | RNDIS (`method`: auto/dhcp/static) or MBIM connect |
| POST | `/api/data/disconnect` | Tear down data path |
| GET | `/api/hotspot` | Hotspot status + tools + WiFi devices |
| POST | `/api/hotspot/start` | Start WPA2 AP + NAT to RNDIS uplink |
| POST | `/api/hotspot/stop` | Stop AP / tear down NAT |
| GET/POST | `/api/usbmode` | Query / set `AT+GTUSBMODE` |
| GET | `/api/history` | Signal samples |
| GET | `/api/mbim` | MBIM helper status |

Hotspot ops guide: [`docs/wifi-hotspot.md`](docs/wifi-hotspot.md) (packages: `hostapd` `dnsmasq` `iw` `iproute2` `nftables`|`iptables`).

---

## Tests

```bash
go test ./...
```

Uses only Go `testing` + temp dirs (no hardware required for unit tests).

---

## License / hardware note

Intended for personal/lab use with Fibocom **FM350-GL** on Linux (e.g. laptop USB). Vendor AT sets and USB modes vary by firmware and OEM SKU; treat AT/USB mode changes carefully.
