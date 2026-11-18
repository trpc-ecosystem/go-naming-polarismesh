[English](README.md) | 中文

# 规则路由

通过 Polaris v2 routing rule 将指定 source service 路由到
`route=canary` 的 provider。

```bash
./examples/route/rule/verify.sh --polaris-server <polaris-server-host>
```

测试同时验证 source service 和 destination label；其他 caller 会正常回退到普通
负载均衡。

脚本会复用同名路由规则，并在需要时重新启用；只有规则不存在时才创建。cleanup
不会删除或禁用路由规则。
