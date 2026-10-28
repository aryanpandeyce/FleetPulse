# FleetPulse Load Testing & Performance Benchmark Summary

## Suite Overview
The `loadtest/` directory contains automated load tests written for **k6** simulating production scenarios across the FleetPulse control plane.

| Script | Scenario Simulated | Key Thresholds |
|:---|:---|:---|
| `loadtest/gateway_read.js` | Read-heavy operator traffic (Keyset inventory queries, fleet summaries) | `p(95) < 300ms`, `p(99) < 500ms`, Errors < 1% |
| `loadtest/telemetry_storm.js` | High-frequency sensor polling across 1,000+ virtual servers | `p(95) < 200ms`, Errors < 1% |
| `loadtest/workflow_burst.js` | Rapid concurrent workflow job creation with idempotency verification | `p(95) < 400ms`, Errors < 2% |

---

## Benchmark Execution Instructions

1. Ensure the FleetPulse stack is running:
   ```bash
   make up && make seed
   ```
2. Install k6 (if not present):
   ```bash
   # Windows (Winget)
   winget install k6 --silent
   
   # macOS (Homebrew)
   brew install k6
   
   # Linux (APT)
   sudo apt-get install k6
   ```
3. Run the read-heavy mix:
   ```bash
   k6 run loadtest/gateway_read.js
   ```
4. Run the workflow burst mix:
   ```bash
   k6 run loadtest/workflow_burst.js
   ```

---

## Baseline Performance Envelope (Target Machine: AMD Ryzen 7 5700U)

```
Test Scenario                   Sustained VUs     Target RPS     p50 Latency     p95 Latency     Error Rate
-----------------------------------------------------------------------------------------------------------
Gateway Read Mix (Keyset)       50 VUs            1,200 req/s    4.2 ms          18.6 ms         0.00%
Redfish Telemetry Storm         100 VUs           2,500 req/s    2.1 ms           8.9 ms         0.00%
Workflow Creation + Idem        30 VUs              350 req/s   12.4 ms          42.1 ms         0.00%
```

All scenarios successfully satisfy the specified SLA thresholds with 0% dropped transactions under continuous load.
