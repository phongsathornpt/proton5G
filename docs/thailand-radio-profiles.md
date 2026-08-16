# Thailand radio profiles (FM350-GL)

This project exposes Thailand-oriented radio presets through the existing `POST /api/rat` endpoint and the Cellular WebUI.

## Profiles

| API mode | Purpose | Preferred `AT+GTACT` command |
|---|---|---|
| `th-nsa` | Thailand 5G NSA / EN-DC with common LTE anchors and NR n41 | `AT+GTACT=17,3,6,101,103,108,128,140,5041` |
| `th-nsa-b40-n41` | Aggressive True-oriented EN-DC profile: LTE B40 anchor + NR n41 | `AT+GTACT=17,3,6,140,5041` |
| `th-lte` | Thailand LTE fallback profile | `AT+GTACT=2,3,3,101,103,108,128,140,141` |
| `th-lte-b40-b41` | Aggressive LTE B40/B41-only profile | `AT+GTACT=2,3,3,140,141` |

Fibocom `+GTACT` uses vendor band IDs. For the bands used here:

- LTE B1 = `101`
- LTE B3 = `103`
- LTE B8 = `108`
- LTE B28 = `128`
- LTE B40 = `140`
- LTE B41 = `141`
- NR n41 = `5041`

The strict profiles can improve scan/selection behavior where the desired cells are known, but they can also remove service completely when those bands are unavailable. Use `Auto mode` or the broader Thailand profile when moving between locations.

## Thailand spectrum rationale

Thailand's 2300 MHz allocation covers 2300-2370 MHz. The June 2025 NBTC auction allocated that block to True Move H Universal Communication. The 2500-2690 MHz range is an IMT allocation and is the relevant mid-band range for NR n41 deployments. For this reason, the strict NSA preset pairs LTE B40 with NR n41 rather than treating LTE B41 as the 5G carrier.

Do not assume the same LTE anchor is available on every site. EN-DC depends on the network's deployed LTE/NR combination, so always verify the live result in:

- `network.endc.anchor_band`
- `network.endc.nr_band`
- `cells[]`
- `ca[]`

## MIMO

MIMO is a negotiated radio capability, not a safe global software switch. Connect the modem's required cellular antenna paths according to the FM350 hardware design and use low-loss, correctly terminated RF paths. The network then determines the active layer count.

`CAComponent` now exposes:

```json
{
  "dl_mimo": "4x4",
  "ul_mimo": "2x2"
}
```

when `AT+GTCAINFO?` reports those values.

A parser correction is included for the FM350 long SCC format. The previous parser could read a DL-MIMO value of `4` as modulation code `4`, falsely displaying `256QAM`. The new enrichment step reads MIMO and modulation from their separate fields.

## Uplink 2CA

The manager detects uplink carrier aggregation from the per-component `ul_active` state parsed from `AT+GTCAINFO?`.

- PCC is treated as the primary uplink carrier.
- An SCC contributes to uplink CA only when the modem reports it as uplink-configured / uplink-active.
- `LTE B40 + LTE B3` with both carriers uplink-active is reported as `Active (LTE 2CC)`.
- `LTE B40 + NR n41` under EN-DC is not labeled same-RAT 2CA. The UI reports both active RATs separately so dual connectivity is not confused with LTE or NR carrier aggregation.

The Cellular panel shows:

- Uplink CA state
- active LTE/NR carrier counts
- UL bands
- UL MIMO per active carrier
- UL modulation per active carrier

This is telemetry, not a force switch. The network and modem firmware decide whether a secondary carrier is actually scheduled for uplink transmission. A strict band lock can reduce UL CA opportunities if it removes the secondary LTE carrier needed by the serving site.

For upload tuning, prefer the broader `th-lte` or `th-nsa` profile first, confirm the live UL carrier combination, then compare stricter profiles at the same location and RF conditions.

## 256QAM

256QAM should not be blindly forced. Modulation is selected dynamically from RF quality, carrier configuration and network scheduling. The manager reports the active modulation from `AT+GTCAINFO?`:

- `QPSK`
- `16QAM`
- `64QAM`
- `256QAM`
- `1024QAM` when firmware reports it

For a useful 256QAM optimization workflow:

1. Choose a profile that reaches the intended B40/n41 site.
2. Improve antenna placement and RF quality rather than forcing a modulation value.
3. Watch RSRP, RSRQ and SINR together with the CA table.
4. Confirm `DL Mod = 256QAM` on the active component before attributing a speed gain to 256QAM.
5. If a strict band profile performs worse, return to `th-nsa` or `auto`; carrier aggregation may need another LTE anchor.

## API examples

Thailand NSA:

```bash
curl -X POST http://localhost:8080/api/rat \
  -H 'Content-Type: application/json' \
  -d '{"mode":"th-nsa"}'
```

Strict B40 + n41 NSA:

```bash
curl -X POST http://localhost:8080/api/rat \
  -H 'Content-Type: application/json' \
  -d '{"mode":"th-nsa-b40-n41"}'
```

Thailand LTE fallback:

```bash
curl -X POST http://localhost:8080/api/rat \
  -H 'Content-Type: application/json' \
  -d '{"mode":"th-lte"}'
```

If the modem firmware rejects a parameterized `+GTACT` command, the existing client fallback applies the corresponding RAT-only command, so the modem is not left without a recovery path.
