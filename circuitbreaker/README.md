# Circuit Breaker Plugin

```yaml
# Circuit breaker is configured under selector plugin.
selector:
  # This selector is based on polaris.
  polaris:
    # Key word for circuit breaker.
    circuitbreaker:
      # Circuit breaker check period, default as 30s.
      checkPeriod: 30s
      # The maximum number of requests allowed after the circuit breaker is half-opened, default as 10.
      requestCountAfterHalfOpen: 10
      # After the circuit breaker is opened, how long does it take to switch to the half-open state, default as 30s.
      sleepWindow: 30s
      # The minimum number of successful requests necessary for the circuit breaker to be closed from half open, default as 8.
      successCountAfterHalfOpen: 8
      # Circuit breaking strategy, default as [errorCount, errorRate].
      chain:
        - errorCount  # Circuit breaker based on error rate.
        - errorRate   # Circuit breaker based on consecutive error count.
      # Config for strategy errorCount.
      errorCount:
        # Threshold to trigger continuous error circuit breaker, default as 10.
        continuousErrorThreshold: 10
        # The minimum count for consecutive errors, default as 10.
        metricNumBuckets: 10
        # Continuous failure period, default as 1m.
        metricStatTimeWindow: 1m0s
      # Config for strategy errorRate.
      errorRate:
        # Threshold to trigger errorRate, default as 0.5.
        errorRateThreshold: 0.5
        # The number of sliding window buckets, default as 5.
        metricNumBuckets: 5
        # The duration of total sliding window, default as 1m.
        metricStatTimeWindow: 1m0s
        # The threshold of requests to trigger errorRate, default as 10.
        requestVolumeThreshold: 10
```
