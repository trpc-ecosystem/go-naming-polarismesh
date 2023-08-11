# 熔断器插件

```yaml
selector:                                          # Configuration for trpc framework service discovery.
  polaris:                                         # Configuration for Polaris Service Discovery.
    circuitbreaker:
      checkPeriod: 30s                             # Instance timing circuit breaker detection period, default value: 30s.
      requestCountAfterHalfOpen: 10                # The maximum number of requests allowed after the circuit breaker is half-opened, default value: 10.
      sleepWindow: 30s                             # After the circuit breaker is opened, how long does it take to switch to the half-open state, default value: 30s.
      successCountAfterHalfOpen: 8                 # The minimum number of successful requests necessary for the circuit breaker to be closed from half open, default value: 8.
      chain:                                       # Circuit breaking strategy, default value: [errorCount, errorRate].
        - errorCount                               # Circuit breaker based on cycle error rate.
        - errorRate                                # Circuit breaker based on cycle consecutive error count.
      errorCount:
        continuousErrorThreshold: 10               # Threshold to trigger continuous error circuit breaker, default value: 10.
        metricNumBuckets: 10                       # The minimum number of statistical units for consecutive errors, default value: 10.
        metricStatTimeWindow: 1m0s                 # Continuous failure statistics period, default value: 1m.
      errorRate:
        errorRateThreshold: 0.5                    # Threshold to trigger error rate fusing, default value: 0.5.
        metricNumBuckets: 5                        # The minimum number of statistical units for error rate fusing, default value: 5.
        metricStatTimeWindow: 1m0s                 # Statistical period of error rate fusing, default value: 1m.
        requestVolumeThreshold: 10                 # The minimum request threshold to trigger the error rate circuit breaker, default value: 10.
```
