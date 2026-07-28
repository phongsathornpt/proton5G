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
         |                   +--> internal/repository  (USB, AT, history, MBIM)
         |
         +--> internal/template    go:embed SPA (HTML/CSS/JS)
         |
         +--> internal/pkg/domain  pure models
```

## Layers & Data Flow

1. **Repository (`internal/repository`)**
   - **USB**: sysfs autosuspend / `power/control`, `USBDEVFS_RESET`.
   - **AT**: serial client, multi-tty probe, parsers (`CSQ`, `CESQ`, `CIMI`, …).
   - **History**: in-memory ring buffer + optional JSON file.
   - **MBIM**: optional `mbimcli` helper.

2. **Usecase (`internal/usecase`)**
   - `ModemService.Status()` aggregates USB + AT, rediscovers ports after failure, hard-resets after streak, samples history.
   - Control: APN, RAT, raw AT, USB reset, MBIM connect/disconnect.
   - Background `RunWatchdog` enforces power without a browser open.

3. **Handler (`internal/handler`)**
   - REST + SSE only; depends on usecase interface, not repositories.
   - Routes: `/api/status`, `/api/events`, `/api/history`, `/api/apn`, `/api/rat`, `/api/raw`, `/api/reset`, `/api/mbim*`.
   - Static UI from `internal/template`.

4. **Domain (`internal/pkg/domain`)**
   - Shared JSON-friendly structs; no I/O.

## Packaging

```bash
go build -o fm350-manager ./cmd/app
```

- Systemd unit: `deploy/fm350-manager.service`
- Default bind: `127.0.0.1:8080`
