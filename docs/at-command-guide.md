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
| RAT Mode (Preferred) | `AT+GTACT?` | `+GTACT: <mode>` (14=5G, 4=LTE, 20=Auto) |
| Force 5G NR | `AT+GTACT=14` | `OK` |
| Force LTE | `AT+GTACT=4` | `OK` |
| Set Auto Mode | `AT+GTACT=20` | `OK` |

### Multi-interface serial

FM350 often exposes several `/dev/ttyUSB*`. The manager probes each candidate with `AT` and uses the first that returns `OK`. Override with `-serial /dev/ttyUSBN` if needed.

## MBIM Data Bearer (optional)

Install `libmbim-utils` for `mbimcli`. The WebUI MBIM panel shells out to:

```bash
mbimcli -d /dev/cdc-wdm0 --simple-connect="apn='your.apn'"
mbimcli -d /dev/cdc-wdm0 --disconnect
```

IP configuration of `wwan0` may still need NetworkManager, `dhcpcd`, or a local script.

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
2. **MBIM device** dropdown (if `/dev/cdc-wdm*` exists) → **Connect** for data bearer.

If MBIM list is empty, the modem is likely **AT-only** composition (no `cdc_mbim`). Signal/SIM/APN still work over the selected AT port.

### Permission denied on `/dev/ttyUSB*`

| Symptom | Fix |
|---------|-----|
| `open serial port … Permission denied` | `sudo` **or** add user to `dialout` and re-login |
| `open /dev/bus/usb/… Permission denied` | hard reset needs root (or custom udev rules) |
| sysfs autosuspend write fails | usually needs root |

Rebuild/install binary + unit: `go build -o fm350-manager ./cmd/app` and `deploy/fm350-manager.service`.
