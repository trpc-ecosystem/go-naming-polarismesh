English | [中文](README.zh_CN.md)

# Circuit Breaker Plugin

```yaml
plugins:
  selector:
    # This selector is based on Polaris.
    polarismesh:
      # Circuit breaker configuration.
      circuitbreaker:
        # Circuit breaker check period, default as 30s.
        checkPeriod: 30s
        # Maximum requests allowed after the circuit breaker is half-opened, default as 10.
        requestCountAfterHalfOpen: 10
        # Time before switching an open breaker to half-open, default as 30s.
        sleepWindow: 30s
        # Minimum successful requests required to close a half-open breaker, default as 8.
        successCountAfterHalfOpen: 8
        # Circuit breaking strategies, default as [errorCount, errorRate].
        chain:
          - errorCount  # Trigger on consecutive errors.
          - errorRate   # Trigger on the error rate.
        # errorCount strategy configuration.
        errorCount:
          # Consecutive error threshold, default as 10.
          continuousErrorThreshold: 10
          # Number of metric buckets, default as 10.
          metricNumBuckets: 10
          # Continuous failure accounting window, default as 1m.
          metricStatTimeWindow: 1m0s
        # errorRate strategy configuration.
        errorRate:
          # Error-rate threshold, default as 0.5.
          errorRateThreshold: 0.5
          # Number of sliding-window buckets, default as 5.
          metricNumBuckets: 5
          # Total sliding-window duration, default as 1m.
          metricStatTimeWindow: 1m0s
          # Minimum request volume before evaluating the error rate, default as 10.
          requestVolumeThreshold: 10
```

Polaris SDK 1.7.x implements `errorCount` and `errorRate` with the `composite` plugin. The legacy
chain names are accepted and normalized to `composite`. Because `composite` has one error-rate
interval, `errorCount.metricStatTimeWindow` must be omitted or match
`errorRate.metricStatTimeWindow`.
Because `composite` always enables both default triggers, configure `errorCount` and `errorRate`
together. A single legacy strategy is rejected during initialization instead of being silently
changed into a dual-strategy breaker.
