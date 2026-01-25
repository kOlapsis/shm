# SHM Go SDK

Privacy-first telemetry client for self-hosted software.

## Installation

```bash
go get github.com/btouchard/shm/sdk/golang
```

## Quick Start

```go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"

    shm "github.com/btouchard/shm/sdk/golang"
)

func main() {
    client, err := shm.New(shm.Config{
        ServerURL:   "https://telemetry.example.com",
        AppName:     "my-app",
        AppVersion:  "1.0.0",
        Environment: "production",
        Enabled:     true,
    })
    if err != nil {
        panic(err)
    }

    // Optional: add custom metrics
    client.SetProvider(func() map[string]interface{} {
        return map[string]interface{}{
            "users_count":    42,  // e.g. getUserCount()
            "jobs_processed": 100, // e.g. getJobsCount()
        }
    })

    // Start telemetry in background
    ctx, cancel := context.WithCancel(context.Background())
    go client.Start(ctx)

    // Graceful shutdown
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    <-sigCh
    cancel()
}
```

## Configuration

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `ServerURL` | `string` | required | Base URL of the SHM server |
| `AppName` | `string` | required | Name of your application |
| `AppVersion` | `string` | required | Version of your application |
| `DataDir` | `string` | `"."` | Directory to store identity file |
| `Environment` | `string` | `""` | Environment identifier (production, staging, etc.) |
| `Enabled` | `bool` | `false` | Enable/disable telemetry |
| `ReportInterval` | `time.Duration` | `1h` | Interval between snapshots (minimum: 1m) |
| `CollectSystemMetrics` | `bool` | `false` | Collect OS/runtime metrics (use `CollectSystemMetricsFromEnv()`) |

## Environment Variables

| Variable | Effect |
|----------|--------|
| `DO_NOT_TRACK=true` or `1` | **Completely disables telemetry** — overrides `Enabled: true` |
| `SHM_COLLECT_SYSTEM_METRICS=false` or `0` | Disables system metrics collection (enabled by default) |

### Example with environment variables

```go
client, _ := shm.New(shm.Config{
    ServerURL:            "https://telemetry.example.com",
    AppName:              "my-app",
    AppVersion:           "1.0.0",
    Enabled:              true,
    CollectSystemMetrics: shm.CollectSystemMetricsFromEnv(), // reads SHM_COLLECT_SYSTEM_METRICS
})
```

> **Note:** If `DO_NOT_TRACK=true` or `DO_NOT_TRACK=1` is set, the client will be disabled regardless of the `Enabled` configuration.

## How It Works

1. **Identity Generation**: On first run, the SDK generates an Ed25519 keypair and a unique instance ID, stored in `{DataDir}/shm_identity.json`

2. **Registration**: The client registers with the server, sending its public key

3. **Activation**: The client activates by sending a signed request

4. **Periodic Snapshots**: System metrics + custom metrics are sent at the configured interval

## System Metrics

The SDK automatically collects:

| Metric | Description |
|--------|-------------|
| `sys_os` | Operating system (linux, darwin, windows) |
| `sys_arch` | Architecture (amd64, arm64) |
| `sys_cpu_cores` | Number of CPU cores |
| `sys_go_version` | Go runtime version |
| `sys_mode` | Deployment mode (kubernetes, docker, standalone) |
| `app_mem_alloc_mb` | Allocated memory in MB |
| `app_goroutines` | Number of goroutines |
| `app_uptime_h` | Application uptime in hours |

## Custom Metrics

Add your own metrics with `SetProvider`:

```go
client.SetProvider(func() map[string]interface{} {
    return map[string]interface{}{
        "db_connections":  5,     // e.g. pool.Stats().OpenConnections
        "cache_hit_rate":  0.95,  // e.g. cache.HitRate()
        "requests_total":  1000,  // e.g. atomic.LoadInt64(&requestCount)
    }
})
```

## Deployment Detection

The SDK automatically detects the deployment environment:

- **Kubernetes**: Detected via `KUBERNETES_SERVICE_HOST` env var or service account
- **Docker**: Detected via `/.dockerenv` or cgroup inspection
- **Standalone**: Default when no container is detected

## Security

- Ed25519 signatures ensure request authenticity
- Private keys never leave the client
- Identity file created with `0600` permissions
- No PII collected by default

## License

MIT License - See [LICENSE](./LICENSE)
