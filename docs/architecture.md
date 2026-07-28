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
   - `ModemService.Status()` aggregates USB + AT, rediscovers ports after failure, hard-resets after streak, samples history.
   - Inventory selection: modem / AT port / RNDIS iface / MBIM device.
   - Control: APN, RAT, raw AT, USB reset, USB composition (`GTUSBMODE`).
   - Data: unified `DataConnect` / `DataDisconnect` (RNDIS or MBIM); legacy MBIM methods remain.
   - Background `RunWatchdog` enforces power without a browser open.

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
