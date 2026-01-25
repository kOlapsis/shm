# SHM Node.js SDK

Privacy-first telemetry client for self-hosted software.

## Requirements

- Node.js >= 22.0.0 (LTS)
- No external dependencies

## Installation

```bash
npm install @btouchard/shm-sdk
```

## Quick Start

```typescript
import { SHMClient } from '@btouchard/shm-sdk';

const client = new SHMClient({
  serverUrl: 'https://telemetry.example.com',
  appName: 'my-app',
  appVersion: '1.0.0',
  environment: 'production',
});

// Optional: add custom metrics
client.setProvider(() => ({
  users_count: getUserCount(),
  jobs_processed: getJobsCount(),
}));

// Start telemetry (returns AbortController)
const controller = client.start();

// To stop later:
controller.abort();
// Or use client.stop()
```

## Configuration

```typescript
interface Config {
  serverUrl: string;        // Base URL of the SHM server (required)
  appName: string;          // Name of your application (required)
  appVersion: string;       // Version of your application (required)
  dataDir?: string;         // Directory to store identity file (default: ".")
  environment?: string;     // Environment identifier (production, staging, etc.)
  enabled?: boolean;        // Enable/disable telemetry (default: true)
  reportIntervalMs?: number; // Interval between snapshots in ms (default: 3600000, min: 60000)
  collectSystemMetrics?: boolean; // Collect OS/runtime metrics (default: true via env)
}
```

## Environment Variables

| Variable | Effect |
|----------|--------|
| `DO_NOT_TRACK=true` or `1` | **Completely disables telemetry** — overrides `enabled: true` |
| `SHM_COLLECT_SYSTEM_METRICS=false` or `0` | Disables system metrics collection (enabled by default) |

### Example with environment variables

```typescript
import { SHMClient, collectSystemMetricsFromEnv } from '@btouchard/shm-sdk';

const client = new SHMClient({
  serverUrl: 'https://telemetry.example.com',
  appName: 'my-app',
  appVersion: '1.0.0',
  enabled: true,
  collectSystemMetrics: collectSystemMetricsFromEnv(), // reads SHM_COLLECT_SYSTEM_METRICS
});
```

> **Note:** If `DO_NOT_TRACK=true` or `DO_NOT_TRACK=1` is set, the client will be disabled regardless of the `enabled` configuration.

## How It Works

1. **Identity Generation**: On first run, the SDK generates an Ed25519 keypair and a unique instance ID, stored in `{dataDir}/shm_identity.json`

2. **Registration**: The client registers with the server, sending its public key

3. **Activation**: The client activates by sending a signed request

4. **Periodic Snapshots**: System metrics + custom metrics are sent at the configured interval

> **Docker/Kubernetes:** The identity file must persist across restarts. Mount a volume for `dataDir`, otherwise a new identity will be generated and the server will reject requests with **401 Unauthorized**.

## System Metrics

The SDK automatically collects:

| Metric | Description |
|--------|-------------|
| `sys_os` | Operating system (linux, darwin, win32) |
| `sys_arch` | Architecture (x64, arm64) |
| `sys_cpu_cores` | Number of CPU cores |
| `sys_node_version` | Node.js version |
| `sys_mode` | Deployment mode (kubernetes, docker, standalone) |
| `app_mem_heap_mb` | Heap memory usage in MB |
| `app_mem_rss_mb` | RSS memory in MB |
| `app_uptime_h` | Application uptime in hours |

## Custom Metrics

Add your own metrics with `setProvider`:

```typescript
// Synchronous provider
client.setProvider(() => ({
  db_connections: 5,        // e.g. pool.totalCount
  cache_hit_rate: 0.95,     // e.g. cache.getHitRate()
  requests_total: 1000,     // e.g. requestCounter
}));

// Async provider supported
client.setProvider(async () => ({
  db_connections: 5,        // e.g. await pool.getStats()
  external_api_status: 1,   // e.g. await checkExternalApi()
}));
```

## Graceful Shutdown

```typescript
const client = new SHMClient(config);
const controller = client.start();

// Using AbortController
process.on('SIGTERM', () => {
  controller.abort();
});

// Or using stop method
process.on('SIGTERM', () => {
  client.stop();
});
```

## Deployment Detection

The SDK automatically detects the deployment environment:

- **Kubernetes**: Detected via `KUBERNETES_SERVICE_HOST` env var or service account
- **Docker**: Detected via `/.dockerenv` or cgroup inspection
- **Standalone**: Default when no container is detected

## Crypto Utilities

The SDK exports crypto utilities for advanced use cases:

```typescript
import { generateKeypair, signMessage, verifySignature } from '@btouchard/shm-sdk';

// Generate new Ed25519 keypair
const { publicKey, privateKey } = generateKeypair();

// Sign a message
const signature = signMessage(privateKey, 'message to sign');

// Verify signature
const isValid = verifySignature(publicKey, 'message to sign', signature);
```

## Security

- Ed25519 signatures ensure request authenticity
- Private keys never leave the client
- Identity file created with `0600` permissions
- No PII collected by default
- Uses native Node.js crypto (no external dependencies)

## TypeScript Support

Full TypeScript support with exported types:

```typescript
import type {
  Config,
  MetricsProvider,
  Identity,
  SystemMetrics,
} from '@btouchard/shm-sdk';
```

## Development

```bash
# Install dependencies
npm install

# Build
npm run build

# Run tests
npm test
```

## License

MIT License - See [LICENSE](./LICENSE)
