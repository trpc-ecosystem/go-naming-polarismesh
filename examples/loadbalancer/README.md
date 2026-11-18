English | [Chinese](README.zh_CN.md)

# Load Balancer

Two providers with different weights verify multiple SDK load balancers.

```bash
./examples/loadbalancer/verify.sh --polaris-server <polaris-server-host>
```

Coverage:

- `polaris_hash` distribution and key stability;
- `polaris_ring_hash` distribution and key stability;
- `polaris_maglev` distribution and key stability;
- `polaris_wr` weighted distribution.

Assertions validate both node coverage and deterministic hash behavior, not just
RPC success.
