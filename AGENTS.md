# AGENTS.md - Project Guidelines for AI Agents

## Overview

Go daemon + embedded WebUI (`fm350-manager`) for the **Fibocom FM350-GL** 5G USB modem on Linux. It is a **cellular router** stack:

| Role | Path |
|------|------|
| **WAN** | FM350 AT monitoring + RNDIS/MBIM data connect |
| **LAN** | Host WiFi software AP (hostapd) + DHCP (dnsmasq) + NAT to LTE uplink |
| **UI** | Single binary SPA (vanilla HTML/CSS/JS), feature-based assets |

Build: `go build -o fm350-manager ./cmd/app`

## Core Rules for Agents

1. **Lazy Senior Developer Principle**: Build what's requested with minimal abstraction. Prefer Go stdlib over extra dependencies.
2. **Architecture** (layered):
   - `cmd/app/`: Entrypoint, flags, wiring, lifecycle only.
   - `internal/pkg/domain/`: Pure models + string/int enums (RAT, reg, tech, PDP, SIM, power, hotspot).
   - `internal/pkg/appdefaults/`: Ports, intervals, recovery thresholds, hotspot defaults, paths.
   - `internal/repository/`: Sysfs USB, serial AT, inventory, history, mbimcli, RNDIS/`ip`, hotspot (hostapd/dnsmasq/nft). Consts in `at_cmd.go` / `paths.go`.
   - `internal/usecase/`: Workflows, recovery, status poller, hotspot orchestration; ports in `ports.go`.
   - `internal/handler/`: HTTP REST, SSE hub, routing (no serial/sysfs).
   - `internal/template/`: Feature-based WebUI — `assets/layout` + `assets/features` + `assets/shared`; `go:embed` + `html/template` (`RenderIndex`).
   - `deploy/`: Systemd unit (root, localhost bind, RuntimeDirectory/StateDirectory).
   - `docs/`: Architecture, AT guide, wifi-hotspot, webui.
3. **No magic strings for closed sets**: Use domain enums (`RATMode`, `RegState`, hotspot states, …). Wire protocol literals only in repository const files. JSON keys/values must stay stable for the WebUI.
4. **Dependency direction**: `cmd/app` → handler/usecase/repository; `handler` → usecase + template (not repository); `usecase` → repository ports; `repository` → domain + external I/O only. **Do not** import `repository` from `usecase` for file helpers that can live in usecase/stdlib.
5. **No unneeded dependencies**: Only `go.bug.st/serial` (+ transitive `golang.org/x/sys`). Prefer stdlib + external CLIs (`mbimcli`, `ip`, `hostapd`, `dnsmasq`, `iw`, `nft`/`iptables`) over new Go modules. Web UI: zero-build-step vanilla HTML/CSS/JS.
6. **Testing**: Non-trivial logic leaves runnable `testing` tests (domain, parsers, inventory, usecase recovery/hotspot, handler httptest, template render smoke).

## Product surfaces (do not break casually)

### Cellular / modem
- Background `RunStatusPoller` owns AT/USB status → cache.
- `CachedStatus` / default `GET /api/status` = no AT I/O; `?fresh=1` forces poll.
- Control: APN, RAT, raw AT, USB reset, `GTUSBMODE` — all via **AT exclusive gate** (`atMu` / `withAT`).
- Inventory: modem/AT/RNDIS/MBIM select; parallel AT probes with TTL cache.

### Data bearer (WAN)
- `DataConnect` / `DataDisconnect`: RNDIS (`ip` + dhclient/dhcpcd) or MBIM (`mbimcli`).
- Auto prefers selected RNDIS net iface.

### WiFi hotspot (LAN)
- Host radio AP → NAT to **RNDIS uplink** (selected net with IPv4).
- Order: Data Connect → Hotspot Start.
- Config persistence: `$STATE_DIRECTORY/hotspot.json` or `/var/lib/fm350-manager/hotspot.json` (mode 0600).
- Diagnostics on status: tools, install_hint, iface driver/operstate/AP known.
- Runtime logs: `/run/fm350-manager/hostapd.log`, `dnsmasq.log`.
- Hotspot I/O is **not** under `atMu` (uses `hotspotMu`).

### WebUI
- Shell: **topbar / sidebar / content / footer** (`assets/layout`).
- Features: overview, cellular, wan, lan, advanced (`assets/features/*`).
- Serve: `GET /` → `template.RenderIndex()`; `GET /assets/*` → embed FS.
- Preserve stable DOM ids used by SSE/status updates when editing markup.
- Docs: `docs/webui.md`, `docs/wifi-hotspot.md`.

## Concurrency (must respect)

```
atMu / withAT   → poll, control AT, rediscover, AT port lifecycle
s.mu            → short cache/selection only (never hold across AT I/O)
hotspotMu       → hotspot start/stop
SSE hub         → one marshal + fan-out; clients only CachedStatus
```

Lock order: **`atMu` → short `s.mu` only** (never `s.mu` then `atMu`).

## Host tools (optional CLIs)

| Tool | Used for |
|------|----------|
| serial (Go module) | AT |
| `ip`, dhclient/dhcpcd | RNDIS |
| `mbimcli` | MBIM (optional) |
| `hostapd`, `dnsmasq`, `iw`, `nft`/`iptables` | Hotspot AP + DHCP + NAT |

When tools are missing, surface **install_hint** (do not silent-fail). Do not auto-`apt install` from the daemon.

## Agent workflow notes

- Prefer editing the correct layer; no “just put it in main”.
- New closed-set values → domain enum + stable JSON.
- New host integration → repository shell-out + usecase port + handler + UI feature folder if user-facing.
- Keep WebUI feature-based: new panels under `assets/features/<name>/`, wire into `content.html` / `base.html` assets list if needed.
- After structural UI changes, ensure `go test ./internal/template` render smoke still passes.
- Do not commit secrets; hotspot password file is 0600 on disk but still sensitive.

## Key docs

| Doc | Topic |
|-----|--------|
| `docs/architecture.md` | Layers, concurrency, data/hotspot path |
| `docs/at-command-guide.md` | AT / USB modes |
| `docs/wifi-hotspot.md` | Packages, NM conflicts, host debug |
| `docs/webui.md` | Template tree, shell regions |
| `plan.md` | Implementation checklist / status |
| `README.md` | Build, flags, API table |
