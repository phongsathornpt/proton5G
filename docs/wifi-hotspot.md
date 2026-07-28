# WiFi Hotspot (LTE uplink + software AP)

## Overview

fm350-monitor can act as a **cellular router**:

1. **WAN**: Fibocom FM350 data path (usually RNDIS `enx…`) via **Data Bearer → Connect**
2. **LAN**: Host WiFi radio as a **WPA2-PSK access point** (hostapd)
3. Clients get DHCP (dnsmasq) on `192.168.50.0/24` and share the LTE uplink via **NAT** (nftables or iptables)

```
Client ──WiFi──► wlan0 (AP) ──NAT──► enx… (FM350) ──LTE──► Internet
```

## Host packages

```bash
# Debian/Ubuntu
sudo apt-get install -y hostapd dnsmasq iw iproute2 iptables
# optional but preferred for NAT cleanup:
sudo apt-get install -y nftables
```

## NetworkManager

If NetworkManager owns `wlan0`, hostapd often fails. Either:

- Use a **USB WiFi stick** left unmanaged, or
- Mark the interface unmanaged:

```ini
# /etc/NetworkManager/conf.d/99-fm350-unmanaged.conf
[keyfile]
unmanaged-devices=interface-name:wlan0
```

Then `systemctl reload NetworkManager`.

Also stop system `hostapd`/`dnsmasq` units if they fight for the same iface:

```bash
sudo systemctl disable --now hostapd dnsmasq 2>/dev/null || true
```

## Hardware

Check AP capability:

```bash
iw list | grep -A20 "Supported interface modes"
# need a line: * AP
```

Prefer **2.4 GHz** (default channel 6). Some Intel cards cannot run 5 GHz AP (LAR firmware limits).

## Usage

1. Start **fm350-manager** as root (systemd unit already does).
2. WebUI: select modem / RNDIS iface → **Data Bearer → Connect**.
3. **WiFi Hotspot**: pick wlan, set SSID + password (8+ chars) → **Start hotspot**.
4. Join from phone; browse the internet through LTE.

API:

| Method | Path |
|--------|------|
| GET | `/api/hotspot` |
| GET | `/api/hotspot/wifi` |
| POST | `/api/hotspot/config` |
| POST | `/api/hotspot/start` |
| POST | `/api/hotspot/stop` |

Runtime files: `$RUNTIME_DIRECTORY` or `/run/fm350-manager/` (`hostapd.conf`, `dnsmasq.conf`, `dnsmasq.leases`, mode `0600`).

### Config persistence

SSID, password, wlan, channel, and `enabled` are saved to:

- `$STATE_DIRECTORY/hotspot.json` under systemd, or
- `/var/lib/fm350-manager/hotspot.json` by default  

File mode `0600` (contains WPA passphrase). Loaded on process start; written on **Save config**, successful **Start**, and **Stop**.

### Associated clients

While the hotspot is running, `GET /api/hotspot` includes `clients[]` merged from:

- `iw dev <wlan> station dump` (associated stations)
- dnsmasq lease file (IP + hostname)

## Teardown

**Stop** in the UI, or process shutdown, removes hostapd/dnsmasq and the `fm350_hotspot` nft table (or iptables rules tagged `fm350_hotspot`).

## Security notes

- WPA2 password required (no open AP).
- WebUI binds to localhost by default in the systemd unit; protect remote access separately.
- Password is redacted as `********` in GET `/api/hotspot`.
