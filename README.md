# fm350-monitor

Lightweight **Linux** Go daemon + embedded WebUI for the **Fibocom FM350-GL** 5G USB modem (`0e8d:7127`).

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
  - **RNDIS** (common on FM350 USB): `ip link up` + DHCP
  - **MBIM** (if present): optional `mbimcli`
- **WiFi hotspot** — host WiFi AP (hostapd) + DHCP (dnsmasq) + NAT to LTE/RNDIS uplink
- **Signal history** — in-memory ring buffer + optional JSON file
- **Single binary** — vanilla HTML/CSS/JS via `go:embed` (no Node build)
- **Auto-elevate** — re-exec with `sudo` when not root (`-no-elevate` to skip)

---

## Required libraries & dependencies

### 1. Build-time (Go modules)

| Module | Role | Required? |
|--------|------|-----------|
| **[go.bug.st/serial](https://github.com/bugst/go-serial)** `v1.8.0` | Serial port open/read/write, USB port enumeration | **Yes** |
| **golang.org/x/sys** | Transitive (serial on Linux) | **Yes** (indirect) |

Everything else is **Go standard library** (`net/http`, `os`, `embed`, `testing`, …).

```bash
# after clone
go mod download   # or: go build ./...
```

`go.mod`:

```text
module fm350-monitor
require go.bug.st/serial v1.8.0
// golang.org/x/sys is indirect
```

### 2. Runtime — system packages (Debian/Ubuntu)

| Package / tool | Purpose | Required? |
|----------------|---------|-----------|
| **Go toolchain** | Build / `go run` | Build only |
| **sudo** | Auto-elevate for sysfs/serial/USB reset | Recommended |
| **iproute2** (`ip`) | RNDIS: `ip link set … up/down` | For RNDIS data connect |
| **isc-dhcp-client** (`dhclient`) **or** **dhcpcd** | RNDIS address acquisition | For RNDIS data connect |
| **libmbim-utils** (`mbimcli`) | MBIM connect/disconnect/query | Only if `/dev/cdc-wdm*` exists |
| **usbutils** (`lsusb`) | Manual debugging | Optional |

Install example (Ubuntu/Debian):

```bash
# Build
sudo apt-get install -y golang-go git

# Runtime helpers (pick DHCP client you prefer)
sudo apt-get install -y iproute2 isc-dhcp-client

# Only needed for true MBIM compositions
sudo apt-get install -y libmbim-utils

# Optional diagnostics
sudo apt-get install -y usbutils
```

Other distros (approximate):

| Distro | Serial stack | DHCP | MBIM CLI |
|--------|--------------|------|----------|
| Fedora/RHEL | (kernel + go serial) | `dhcp-client` / `dhcpcd` | `libmbim` / `libmbim-utils` |
| Arch | same | `dhclient` / `dhcpcd` | `libmbim` |

### 3. Kernel modules / drivers (usually auto-loaded)

| Module / driver | When |
|-----------------|------|
| **option** + **usb_wwan** + **usbserial** | Multi-serial AT ports (`/dev/ttyUSB*`) |
| **rndis_host** | RNDIS data (network iface e.g. `enx…`) — **common FM350 USB mode** |
| **cdc_mbim** | MBIM control node `/dev/cdc-wdm*` — **only if composition is MBIM** |

No out-of-tree driver is required by this project.

### 4. Device nodes & privileges

| Resource | Access |
|----------|--------|
| `/dev/ttyUSB*` | AT — root **or** group **`dialout`** |
| `/sys/module/usbcore/parameters/autosuspend` | Write — typically **root** |
| `/sys/bus/usb/devices/…/power/control` | Write — typically **root** |
| `/dev/bus/usb/BBB/DDD` | `USBDEVFS_RESET` — typically **root** |
| `/dev/cdc-wdm*` | MBIM — root or udev rules |
| `ip` / DHCP | RNDIS connect — typically **root** |

```bash
# Non-root serial only (no hard reset / limited sysfs):
sudo usermod -aG dialout "$USER"   # then re-login
```

Default app behavior: if not root, **re-runs under `sudo`** (pass `-no-elevate` to disable).

### 5. Not required

- Node.js / npm / webpack (UI is embedded vanilla JS)
- ModemManager / NetworkManager (optional on the host; not used by the daemon)
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
| POST | `/api/data/connect` | RNDIS DHCP or MBIM connect |
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
