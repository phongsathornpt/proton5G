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

## Optional / Later
- [ ] **Netlink IP config**: Apply DHCP/static to `wwan0` after MBIM connect without external scripts.
- [ ] **Fibocom proprietary signal**: e.g. `AT+GTCCINFO` if CESQ unavailable on a firmware build.
- [ ] **USB composition switch**: create `cdc-wdm` when modem is AT-only (vendor-specific).

## UI modem selection
- [x] Inventory API `GET /api/modems`, select `POST /api/modems/select`
- [x] Dropdown: modem list + AT port + MBIM device
- [x] Empty MBIM messaging when no `/dev/cdc-wdm*`

