# Architecture Overview - FM350-GL Modem Manager

## Overview
This application provides a Go service and single-binary WebUI to monitor and manage the Fibocom FM350-GL 5G USB modem on Linux systems.

## Component Structure

```
cmd/app/main.go  --> flags, wiring, lifecycle
         |
         +--> internal/handler     HTTP / SSE / static routes
         |         |
         |         +--> internal/usecase   ModemService workflows
         |                   |
         |                   +--> internal/repository
         |                         (USB, AT, history, inventory, MBIM, RNDIS/net)
         |
         +--> internal/template    go:embed SPA (HTML/CSS/JS)
         |
         +--> internal/pkg/domain  pure models + enums
```

## Layers & Data Flow

1. **Repository (`internal/repository`)**
   - **USB**: sysfs autosuspend / `power/control`, `USBDEVFS_RESET`.
   - **AT**: serial client, multi-tty probe, parsers (`CSQ`, `CESQ`, `GTCAINFO`/`GTCCINFO`, `GTUSBMODE`, …).
   - **Inventory**: USB tree → AT ports, MBIM (`cdc-wdm*`), net ifaces (RNDIS/ECM + IPv4 labels).
   - **History**: in-memory ring buffer + optional JSON file.
   - **MBIM**: optional `mbimcli` helper.
   - **Net**: RNDIS path via `ip link` + `dhclient`/`dhcpcd`; `ip -4 addr` for labels.

2. **Usecase (`internal/usecase`)**
   - **Background `RunStatusPoller`** (one goroutine) owns AT/USB sampling and recovery; writes a cache.
   - `CachedStatus()` — hot path for SSE / default `GET /api/status` (no AT I/O).
   - `Status()` / `?fresh=1` — force one poll (tests, manual refresh).
   - Inventory selection: modem / AT port / RNDIS iface / MBIM device.
   - Control: APN, RAT, raw AT, USB reset, USB composition (`GTUSBMODE`).
   - Data: unified `DataConnect` / `DataDisconnect` (RNDIS or MBIM); legacy MBIM methods remain.
   - Background `RunWatchdog` enforces USB presence/power without a browser open.

### Concurrency model

```
main
  ├─ go RunWatchdog(ctx)          // USB only
  ├─ go RunStatusPoller(ctx)      // status samples → cache (via atMu)
  ├─ go SSEHub.Run(ctx)           // marshal cache once → fan-out
  ├─ go history saver (optional)
  └─ go http.Server
        ├─ GET /api/events × N  → subscribe to hub (no AT)
        ├─ GET /api/status → cache (or Status if ?fresh=1)
        └─ POST control → atMu → AT (same exclusive section as poller)
```

- **SSE hub** (`internal/handler/sse_hub.go`): one ticker marshals `CachedStatus()` and non-blocking-sends to subscriber channels; slow clients drop frames. Wire format unchanged (`data: <FullStatus JSON>`).
- **AT work queue (`atMu` / `withAT`)**: single exclusive section for manager-port AT work — status poll, recovery/rediscover, control (`SetAPN`/`SetRAT`/`RawAT`/`USBMode`/`SetUSBMode`), and port lifecycle (`Close`/`SetPortName` on select/reset). FIFO via mutex waiters; HTTP still blocks until the job finishes. Prevents control from interleaving mid-`GetFullStatus` or racing rediscover.
- Lock order: **`atMu` → short `s.mu` only** (never `s.mu` then `atMu`). `ModemService.mu` is not held across AT I/O.
- `at.Client` still mutexes each serial write/read; usecase `atMu` is the multi-command / lifecycle gate.
- **Inventory / discover**: `ProbeATPortsCached` probes distinct `ttyUSB*` in parallel (bounded, default 6), with a 15s TTL cache; skips the manager’s open AT port. Rediscover runs only under `atMu` after `Close`.

3. **Handler (`internal/handler`)**
   - REST + SSE only; depends on usecase interface, not repositories.
   - Routes:
     - Status: `/api/status`, `/api/events`, `/api/history`
     - Inventory: `/api/modems`, `/api/modems/select`
     - Control: `/api/apn`, `/api/rat`, `/api/raw`, `/api/reset`
     - Data: `/api/data/connect`, `/api/data/disconnect`
     - USB composition: `/api/usbmode` (GET/POST, `AT+GTUSBMODE`)
     - MBIM (legacy): `/api/mbim`, `/api/mbim/connect`, `/api/mbim/disconnect`
   - Optional API token (`-token` / `FM350_API_TOKEN`): Bearer, `X-API-Token`, or `?token=` (SSE).
   - Static UI from `internal/template`.

4. **Domain (`internal/pkg/domain`)**
   - Shared JSON-friendly structs and enums; no I/O.
   - Inventory / data modes: `rndis`, `mbim`, `mixed`, `at_only`, `none`.

## Data path decision

| Modem composition | Kernel | Connect path |
|-------------------|--------|--------------|
| RNDIS + serial (common FM350) | `rndis_host` + `option` | `ip link up` + DHCP on net iface |
| MBIM | `cdc_mbim` | `mbimcli` on `/dev/cdc-wdm*` |
| AT-only | serial only | monitoring only; no data UI targets |

`DataConnect` auto mode prefers selected RNDIS iface, else selected MBIM device.

## Packaging

```bash
go build -o fm350-manager ./cmd/app
```

- Systemd unit: `deploy/fm350-manager.service` (binds `127.0.0.1` by default)
- Process default bind: `0.0.0.0:8080` (CLI); prefer localhost for unattended installs
