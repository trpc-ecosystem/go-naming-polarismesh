English | [Chinese](README.zh_CN.md)

# Multi Context

Creates `polaris-a` and `polaris-b` selector configurations in one process, each
with an independent Polaris SDKContext and persistence directory.

```bash
./examples/multiContext/verify.sh --polaris-server <polaris-server-host>
```

`polaris-a://` resolves service A and `polaris-b://` resolves service B. The test
checks selector names, service names and selected addresses to demonstrate
isolation.
