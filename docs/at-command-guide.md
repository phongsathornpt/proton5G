# FM350-GL AT Command Guide & Troubleshooting

## USB Autosuspend Fix

When plugged in via USB on Linux, the FM350-GL may disconnect after ~2 seconds due to kernel power management.

To fix manually:
```bash
echo -1 | sudo tee /sys/module/usbcore/parameters/autosuspend
echo on | sudo tee /sys/devices/.../power/control
```

The Go daemon automatically performs these writes at startup and on the background watchdog interval.

## USB Hard Reset

If the modem is present in sysfs but AT stops responding, the manager may issue `USBDEVFS_RESET`:

```bash
# Equivalent idea (device path from sysfs busnum/devnum):
# ioctl(USBDEVFS_RESET) on /dev/bus/usb/BBB/DDD
```

Or via the WebUI **USB Hard Reset** button / `POST /api/reset` (requires privileges on `/dev/bus/usb`).

## Common AT Commands

| Function | Command | Expected Output |
|---|---|---|
| Check Signal | `AT+CSQ` | `+CSQ: <rssi>,99` |
| Extended Signal | `AT+CESQ` | `+CESQ: …,<rsrq>,<rsrp>` |
| Check Operator | `AT+COPS?` | `+COPS: 0,0,"Operator",13` |
| 5G Registration | `AT+C5GREG?` | `+C5GREG: 0,1` (1=Home, 5=Roaming) |
| LTE Registration | `AT+CEREG?` | `+CEREG: 0,1` |
| SIM Status | `AT+CPIN?` | `+CPIN: READY` |
| IMSI | `AT+CIMI` | `31026…` then `OK` |
| ICCID | `AT+CCID` / `AT+ICCID` | `+CCID: "89…"` |
| APN Context | `AT+CGDCONT?` | `+CGDCONT: 1,"IPV4V6","internet"` |
| Set APN | `AT+CGDCONT=1,"IPV4V6","your.apn"` | `OK` |
| RAT Mode (Preferred) | `AT+GTACT?` | `+GTACT: <mode>` (17=EN-DC, 14=5G SA, 4=LTE, 20=Auto) |
| Force 5G NSA (EN-DC) | `AT+GTACT=17,3,6,0` / `AT+GTACT=17` | `OK` |
| Force 5G SA | `AT+GTACT=14,6,6,0` / `AT+GTACT=14` | `OK` |
| Force LTE | `AT+GTACT=2,3,3,0` / `AT+GTACT=4` | `OK` |
| Set Auto Mode | `AT+GTACT=20,6,3,0` / `AT+GTACT=20` | `OK` |
| 5G Option (SA/NSA) | `AT+E5GOPT?` / `AT+E5GOPT=5` | 5=NSA only, 6=SA only, 7=SA+NSA |
| Activate PDP | `AT+CGACT=1,1` | `OK` (ERROR if already active is ignored when `CGPADDR` has an IP) |
| PDP address | `AT+CGPADDR=1` | `+CGPADDR: 1,"10.x.x.x"` |
| PDP DNS | `AT+GTDNS=1` | `+GTDNS: 1,"…","…"` |
| Temperature | `AT+GTSENRDTEMP=1` | `+GTSENRDTEMP: 1,<milli-°C>` |
| Cells | `AT+GTCCINFO?` | serving + neighbors |
| Carrier aggregation | `AT+GTCAINFO?` | `PCC:` / `SCC:` lines |
| Identity | `AT+CGMI` / `CGMM` / `CGMR` / `CGSN` | manufacturer / model / FW / IMEI |

### Multi-interface serial

FM350 often exposes several `/dev/ttyUSB*`. The manager probes candidates with `AT` **in parallel** (bounded; 15s cache) and prefers USB VID/PID matches, then the first port that returns `OK`. The inventory UI labels ports `(AT OK)`. Override with `-serial /dev/ttyUSBN` if needed. The open monitoring port is never re-probed while held.

## Extended / proprietary signal

Status polling uses:

1. `AT+CSQ` (RSSI)
2. `AT+CESQ` — **3GPP** `+CESQ: <rxlev>,<ber>,<rscp>,<ecno>,<rsrq>,<rsrp>` (RSRP = raw−140). This is not the vendor-GUI field order.
3. `AT+GTCAINFO?` / `AT+GTCCINFO?` every poll for cells, CA, and RSRP/SINR fallback
4. `AT+GTSENRDTEMP=1` (temperature)
5. `AT+CGPADDR` / `AT+GTDNS` (PDP IP + DNS; gateway is guessed as `a.b.c.1`)
6. Identity (`CGMI`/`CGMM`/`CGMR`/`CGSN`) once until IMEI is cached

`AT+CESQ` has **no SINR**. SINR comes from `GTCCINFO` (0.5 dB units → integer dB).

## USB composition (`AT+GTUSBMODE`)

Documented stock modes for many FM350 USB builds:

| Mode | Meaning |
|------|---------|
| **40** | RNDIS + AT + GNSS + META + DEBUG + NPT + ADB |
| **41** | Default; mode 40 + extra AP(LOG)/AP(META) serials |

Both are **RNDIS**, not MBIM. Setting a mode re-enumerates USB (brief disconnect). WebUI: **USB Composition** card or `GET/POST /api/usbmode`.

```bash
AT+GTUSBMODE?
AT+GTUSBMODE=41
```

## MBIM Data Bearer (optional)

Install `libmbim-utils` for `mbimcli`. The WebUI data path shells out to:

```bash
mbimcli -d /dev/cdc-wdm0 --simple-connect="apn='your.apn'"
mbimcli -d /dev/cdc-wdm0 --disconnect
```

RNDIS **Connect** (default `method=auto`):

1. `AT+CGACT=1,<cid>` then `AT+CGPADDR` / `AT+GTDNS`
2. `ip link up` + `dhclient`/`dhcpcd`
3. If the iface has no IPv4 and the modem reported a PDP address: assign `CGPADDR/24` and add `default via a.b.c.1 metric 100`

`method=dhcp` skips static. `method=static` skips DHCP. The daemon does **not** rewrite `/etc/resolv.conf` or stop ModemManager. PDP DNS is shown in the WAN panel only.

## Run

```bash
# Preferred package path (not main.go alone — package may have multiple files):
go run ./cmd/app

# Auto-elevates with sudo when not root (prompts for password).
# Equivalent manual forms:
sudo go run ./cmd/app
go run ./cmd/app -no-elevate          # stay unprivileged (expect permission errors)

# Non-root serial only (no hard reset / limited sysfs):
sudo usermod -aG dialout "$USER"     # then log out/in
go run ./cmd/app -no-elevate
```

### Modem list UI

The dashboard **Modem** card lists discovered USB/serial devices:

1. Choose **Device** and **AT port** → **Use selected** (binds AT monitoring).
2. **Data Bearer** card lists **RNDIS** network interfaces (e.g. `enx…`) and/or **MBIM** (`/dev/cdc-wdm*`).

#### Why “MBIM device none found” on FM350-GL

Many FM350 USB compositions are **RNDIS + multi-serial**, not MBIM:

| Composition | Kernel | Data UI |
|-------------|--------|---------|
| RNDIS (common) | `rndis_host` + `option` (`ttyUSB*`) | Use **RNDIS** iface (e.g. `enx000011121314`) |
| MBIM | `cdc_mbim` → `/dev/cdc-wdm0` | Use MBIM + `mbimcli` |

Apply APN with AT first, then **Data Bearer → Connect** (RNDIS: `ip link up` + DHCP).

### Permission denied on `/dev/ttyUSB*`

| Symptom | Fix |
|---------|-----|
| `open serial port … Permission denied` | `sudo` **or** add user to `dialout` and re-login |
| `open /dev/bus/usb/… Permission denied` | hard reset needs root (or custom udev rules) |
| sysfs autosuspend write fails | usually needs root |

Rebuild/install binary + unit: `go build -o fm350-manager ./cmd/app` and `deploy/fm350-manager.service`.
