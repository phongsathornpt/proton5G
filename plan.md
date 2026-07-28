# FM350-GL Modem Manager - Implementation Plan & Status

## Overview
A lightweight Go daemon and embedded WebUI to monitor, manage, and prevent disconnects for the Fibocom FM350-GL 5G USB modem on Linux (Lenovo Yoga 520).

## Completed Work
- [x] Domain models (`internal/pkg/domain`)
- [x] Domain enums: RATMode/RATModePref, RegState, RadioTech, SIMState, PDPType, PowerControl, DefaultFM350
- [x] App defaults package (`internal/pkg/appdefaults`)
- [x] Repository consts: `at_cmd.go`, `paths.go`; parsers/client return enums
- [x] Handler validation: invalid RAT/PDP → HTTP 400
- [x] Repository layer: USB watchdog, AT serial, history ring/file, MBIM helper
- [x] Usecase layer: status, recovery, control, background watchdog
- [x] Handler + embedded template UI (REST, SSE)
- [x] Entrypoint `cmd/app` with flags `-port`, `-bind`, `-serial`, `-watch`, `-history`, `-history-file`
- [x] Systemd unit (`deploy/fm350-manager.service`)
- [x] Layered structure refactor (handler / usecase / repository / pkg / template)

## UI modem selection
- [x] Inventory API `GET /api/modems`, select `POST /api/modems/select`
- [x] Dropdown: modem list + AT port + MBIM device
- [x] Empty MBIM messaging when no `/dev/cdc-wdm*`

## Data bearer (RNDIS + MBIM)
- [x] Discover net ifaces under USB (`netIfacesUnder`) and surface in inventory
- [x] Domain: `data_mode`, `net_ifaces`, `selected_net`, `DataConnectRequest`
- [x] Repository: `NetRepo` / `ConnectRNDIS` (`ip` + dhclient/dhcpcd)
- [x] Usecase: unified `DataConnect` / `DataDisconnect` (auto prefers RNDIS)
- [x] Handler: `POST /api/data/connect`, `POST /api/data/disconnect`
- [x] WebUI Data Bearer card (RNDIS + MBIM select, Connect/Disconnect)
- [x] Tests: usecase data path, handler data endpoints, net helpers
- [x] Net iface IPv4 labels via `ip -4 -o addr` (no pure netlink/DHCP client yet)

## Signal
- [x] CESQ merge into CSQ
- [x] Fibocom proprietary fallback: `AT+GTCAINFO?` / `AT+GTCCINFO?` when RSRP empty

## USB composition
- [x] `AT+GTUSBMODE?` / `AT+GTUSBMODE=<n>` via GET/POST `/api/usbmode`
- [x] WebUI USB Composition card (modes 40/41 documented as RNDIS, not MBIM)
- [x] Note: stock FM350 USB firmware does **not** expose MBIM via 40/41

## Hardening
- [x] Optional API token (`-token` / `FM350_API_TOKEN`)
- [x] Systemd unit binds `127.0.0.1` by default; optional `EnvironmentFile` for token

## Concurrency (goroutines)
- [x] Background `RunStatusPoller` owns AT status + recovery; cache for SSE
- [x] Short `ModemService.mu` critical sections (no lock across AT I/O)
- [x] `pollMu` serializes concurrent `Status()` / poller
- [x] SSE + default `GET /api/status` use `CachedStatus`; `?fresh=1` forces poll
- [x] `main`: WaitGroup for watchdog / poller / history; clean shutdown
- [x] Flag `-poll` (default 2s)
- [x] Parallel AT inventory probes (`ProbeATPortsCached`, ListModems)
- [x] Parallel DiscoverATPort (same helper; list-order preference preserved)
- [x] Optional SSE broadcast hub (`SSEHub`: one marshal + fan-out; `Server.Run` from main)
- [ ] Optional AT work queue for control cmds (only if contention shows up)

## Optional / Later
- [ ] **Native netlink DHCP/static**: replace `dhclient`/`dhcpcd` shell-out with pure Go/netlink (large; still prefer CLI per AGENTS.md unless needed)
- [ ] **True MBIM composition**: only if a firmware mode document lists a non-RNDIS profile for this SKU
- [ ] **TLS / reverse-proxy examples** for remote access (token alone is not enough over the internet)
