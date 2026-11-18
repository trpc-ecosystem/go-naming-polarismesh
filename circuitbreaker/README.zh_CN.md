[English](README.md) | 中文

# 熔断插件

```yaml
# 熔断通过 selector 插件配置。
selector:
  # 该 selector 基于北极星。
  polarismesh:
    # 熔断配置关键字。
    circuitbreaker:
      # 熔断检查周期，默认 30s。
      checkPeriod: 30s
      # 熔断半开后允许的最大请求数，默认 10。
      requestCountAfterHalfOpen: 10
      # 熔断打开后切换到半开状态的等待时间，默认 30s。
      sleepWindow: 30s
      # 从半开切换为关闭所需的最小成功请求数，默认 8。
      successCountAfterHalfOpen: 8
      # 熔断策略，默认 [errorCount, errorRate]。
      chain:
        - errorCount  # 基于连续错误数的熔断器。
        - errorRate   # 基于错误率的熔断器。
      # errorCount 策略配置。
      errorCount:
        # 触发连续错误熔断的阈值，默认 10。
        continuousErrorThreshold: 10
        # 连续错误统计的最小计数，默认 10。
        metricNumBuckets: 10
        # 连续失败统计周期，默认 1m。
        metricStatTimeWindow: 1m0s
      # errorRate 策略配置。
      errorRate:
        # 触发 errorRate 熔断的错误率阈值，默认 0.5。
        errorRateThreshold: 0.5
        # 滑动窗口桶数量，默认 5。
        metricNumBuckets: 5
        # 总滑动窗口时长，默认 1m。
        metricStatTimeWindow: 1m0s
        # 触发错误率统计所需的最小请求数，默认 10。
        requestVolumeThreshold: 10
```

Polaris SDK 1.7.x 使用 `composite` 插件实现 `errorCount` 和 `errorRate`。
旧策略名仍可接受，并会被归一化为 `composite`。由于 `composite` 只有一个错误率统计区间，
`errorCount.metricStatTimeWindow` 必须省略，或与 `errorRate.metricStatTimeWindow`
保持一致。

`composite` 总是同时启用两个默认 trigger，因此应同时配置 `errorCount` 和 `errorRate`。
初始化时会拒绝单独配置一个旧策略，不会静默转换成双策略熔断器。
