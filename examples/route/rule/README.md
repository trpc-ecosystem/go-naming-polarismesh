English | [Chinese](README.zh_CN.md)

# Rule Route

Creates a Polaris v2 routing rule that sends one source service to providers
labeled `route=canary`.

```bash
./examples/route/rule/verify.sh --polaris-server <polaris-server-host>
```

The test proves both source service and destination label matching. Another
caller falls back to normal load balancing.

The script reuses an existing same-name rule and enables it when necessary.
It creates the rule only when no matching rule exists. Cleanup leaves routing
rules untouched.
