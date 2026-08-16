# Live downlink capacity estimate

The Cellular panel derives a radio-side downlink ceiling from the same `AT+GTCAINFO?` carrier snapshot exposed by `/api/status` and SSE.

## API

`FullStatus` includes a `downlink_capacity` object:

```json
{
  "active_cc": 3,
  "lte_cc": 2,
  "nr_cc": 1,
  "bandwidth_known_cc": 3,
  "total_bandwidth_mhz": 130,
  "lte_bandwidth_mhz": 40,
  "nr_bandwidth_mhz": 90,
  "bands": ["B40", "B3", "n41"],
  "best_dl_mimo": "4x4",
  "best_dl_modulation": "256QAM",
  "estimated_peak_mbps": 2912,
  "estimated_from_cc": 3,
  "estimate_complete": true,
  "device_ceiling_mbps": 4670,
  "limiter": "reported CA / radio configuration"
}
```

## Estimate

Only carriers with all three inputs contribute to the estimated peak:

- downlink bandwidth in MHz
- active downlink MIMO layers
- active downlink modulation

The current heuristic is:

```text
carrier Mbps = bandwidth MHz × modulation bits/symbol × MIMO layers × 0.70
estimated peak = sum(carrier Mbps), capped at 4670 Mbps
```

The modulation mapping is QPSK=2, 16QAM=4, 64QAM=6, 256QAM=8 and 1024QAM=10 bits/symbol.

The `0.70` factor intentionally keeps the result below a raw modulation/layer multiplication and represents a coarse allowance for OFDM/control overhead plus duplex/scheduling effects. It is not a 3GPP peak-throughput formula.

If one or more reported carriers are missing bandwidth, MIMO or modulation, `estimate_complete` is false and `estimated_from_cc` shows how many carriers contributed. The UI labels that result as a partial estimate.

## Thailand examples

A True-oriented NSA snapshot with:

```text
B40  20 MHz  4x4  256QAM
B3   20 MHz  4x4  256QAM
n41  90 MHz  4x4  256QAM
```

produces 130 MHz total reported bandwidth and a 2912 Mbps heuristic radio ceiling.

An n41-only 100 MHz carrier at 4x4 / 256QAM produces 2240 Mbps.

These values are estimates, not guaranteed speed-test results. Real throughput can be lower because of the operator/SIM package cap, TDD DL/UL slot pattern, scheduler load, RF quality, backhaul, protocol overhead, thermal behavior and the serving site's supported CA combination.

## FM350-GL ceiling

The estimator caps its result at `4670 Mbps`, matching the FM350-GL/FM350 public theoretical Sub-6 downlink specification used by this project. The card shows that device ceiling separately from the live radio estimate so a weak or narrow carrier set is not confused with modem capability.
