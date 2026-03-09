# <img src="./logos/shm-logo.svg" width="32"> SHM API Reference

This document describes the REST API used by instances to report telemetry data to the SHM server.

## Overview

SHM uses Ed25519 cryptographic signatures for request authentication. Each instance generates a keypair on first run and registers its public key with the server.

### Base URL

```
https://your-shm-server.example.com
```

### Authentication Flow

1. **Register** - Instance sends its public key (unauthenticated)
2. **Activate** - Instance proves ownership by signing a request
3. **Snapshot** - Instance periodically sends signed metrics

## Endpoints

### GET /api/v1/healthcheck

Health check endpoint for liveness probes. No authentication, no rate limiting.

**Response:**

```json
{
  "status": "ok"
}
```

**Status Codes:**

| Code | Description |
|------|-------------|
| 200 | Server is healthy |

Use this for Docker/Kubernetes probes:

```yaml
livenessProbe:
  httpGet:
    path: /api/v1/healthcheck
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10
```

---

### POST /v1/register

Register a new instance with the server. This is the only unauthenticated endpoint.

**Request Body:**

```json
{
  "instance_id": "unique-uuid-v4",
  "public_key": "hex-encoded-ed25519-public-key",
  "app_name": "my-app",
  "app_version": "1.2.0",
  "deployment_mode": "docker",
  "environment": "production",
  "os_arch": "linux/amd64"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `instance_id` | string | Yes | Unique identifier (UUID v4 recommended) |
| `public_key` | string | Yes | Ed25519 public key, hex-encoded (64 chars) |
| `app_name` | string | Yes | Name of your application |
| `app_version` | string | Yes | Version string |
| `deployment_mode` | string | No | How the app is deployed (docker, binary, kubernetes...) |
| `environment` | string | No | Environment name (production, staging, dev...) |
| `os_arch` | string | No | OS and architecture (linux/amd64, darwin/arm64...) |

**Response:**

```json
{
  "status": "ok",
  "message": "Registered"
}
```

**Status Codes:**

| Code | Description |
|------|-------------|
| 201 | Instance registered successfully |
| 400 | Invalid JSON body |
| 405 | Method not allowed (use POST) |
| 500 | Server error |

**curl Example:**

```bash
curl -X POST https://shm.example.com/v1/register \
  -H "Content-Type: application/json" \
  -d '{
    "instance_id": "550e8400-e29b-41d4-a716-446655440000",
    "public_key": "a1b2c3d4e5f6...",
    "app_name": "my-app",
    "app_version": "1.0.0",
    "environment": "production"
  }'
```

---

### POST /v1/activate

Activate a registered instance. This proves the instance owns the private key corresponding to the registered public key.

**Headers:**

| Header | Required | Description |
|--------|----------|-------------|
| `X-Instance-ID` | Yes | The instance_id used during registration |
| `X-Signature` | Yes | Ed25519 signature of the request body, hex-encoded |

**Request Body:**

The body can be empty (`{}`) or contain any valid JSON. The signature is computed over the exact body bytes.

```json
{}
```

**Response:**

```json
{
  "status": "active",
  "message": "Instance activated successfully"
}
```

**Status Codes:**

| Code | Description |
|------|-------------|
| 200 | Instance activated |
| 401 | Missing authentication headers |
| 403 | Invalid signature or unknown instance |
| 405 | Method not allowed |
| 500 | Server error |

---

### POST /v1/snapshot

Send a metrics snapshot. Must be called periodically (recommended: every 60 seconds).

**Headers:**

| Header | Required | Description |
|--------|----------|-------------|
| `X-Instance-ID` | Yes | The instance_id |
| `X-Signature` | Yes | Ed25519 signature of the request body |

**Request Body:**

```json
{
  "instance_id": "unique-uuid-v4",
  "timestamp": "2024-01-15T10:30:00Z",
  "metrics": {
    "users_count": 150,
    "documents_count": 1234,
    "cpu_percent": 12.5,
    "memory_mb": 512,
    "custom_metric": 42
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `instance_id` | string | Yes | The instance_id |
| `timestamp` | string | Yes | ISO 8601 timestamp |
| `metrics` | object | Yes | Arbitrary key-value metrics (schema-agnostic) |

The `metrics` field accepts any JSON object. You define what metrics matter for your application.

**Response:**

```json
{
  "status": "ok",
  "message": "Snapshot received"
}
```

**Status Codes:**

| Code | Description |
|------|-------------|
| 202 | Snapshot accepted |
| 400 | Invalid JSON |
| 401 | Missing authentication headers |
| 403 | Invalid signature |
| 405 | Method not allowed |
| 500 | Server error |

---

## Cryptographic Signature

SHM uses Ed25519 for request signing. Here's how to implement it:

### Key Generation

Generate a 32-byte seed, then derive the keypair:

```
seed = random(32 bytes)
(publicKey, privateKey) = ed25519.GenerateKey(seed)
```

The public key is 32 bytes. Encode it as hex (64 characters) for the API.

### Signing a Request

1. Serialize the request body as JSON bytes
2. Sign the bytes with Ed25519: `signature = ed25519.Sign(privateKey, bodyBytes)`
3. Hex-encode the signature (128 characters)
4. Set `X-Signature` header to the hex-encoded signature

### Verification (Server-side)

The server reconstructs the signature verification:

```
pubKey = hex.Decode(instance.public_key)
signature = hex.Decode(request.Header["X-Signature"])
valid = ed25519.Verify(pubKey, request.Body, signature)
```

### Example (Go)

```go
import (
    "crypto/ed25519"
    "encoding/hex"
    "encoding/json"
)

// Generate keypair (once, on first run)
publicKey, privateKey, _ := ed25519.GenerateKey(nil)
publicKeyHex := hex.EncodeToString(publicKey)

// Sign a request
body := map[string]any{"instance_id": instanceID, "timestamp": time.Now()}
bodyBytes, _ := json.Marshal(body)
signature := ed25519.Sign(privateKey, bodyBytes)
signatureHex := hex.EncodeToString(signature)

// Set headers
req.Header.Set("X-Instance-ID", instanceID)
req.Header.Set("X-Signature", signatureHex)
```

### Example (Python)

```python
from nacl.signing import SigningKey
import json

# Generate keypair (once, on first run)
signing_key = SigningKey.generate()
public_key_hex = signing_key.verify_key.encode().hex()

# Sign a request
body = {"instance_id": instance_id, "timestamp": "2024-01-15T10:30:00Z"}
body_bytes = json.dumps(body, separators=(',', ':')).encode()
signature = signing_key.sign(body_bytes).signature.hex()

# Set headers
headers = {
    "X-Instance-ID": instance_id,
    "X-Signature": signature,
    "Content-Type": "application/json"
}
```

### Example (Node.js)

```javascript
import { generateKeyPairSync, sign } from 'crypto';

// Generate keypair (once, on first run)
const { publicKey, privateKey } = generateKeyPairSync('ed25519');
const publicKeyHex = publicKey.export({ type: 'spki', format: 'der' })
    .slice(-32).toString('hex');

// Sign a request
const body = JSON.stringify({ instance_id: instanceId, timestamp: new Date().toISOString() });
const signature = sign(null, Buffer.from(body), privateKey).toString('hex');

// Set headers
const headers = {
    'X-Instance-ID': instanceId,
    'X-Signature': signature,
    'Content-Type': 'application/json'
};
```

---

## Error Responses

All errors return a plain text message with an appropriate HTTP status code:

| Status | Message | Cause |
|--------|---------|-------|
| 400 | `Invalid JSON` | Malformed request body |
| 401 | `Missing authentication headers` | Missing X-Instance-ID or X-Signature |
| 403 | `Unauthorized` | Instance not found |
| 403 | `Invalid signature` | Signature verification failed |
| 405 | `Method not allowed` | Wrong HTTP method |
| 500 | `Server error` | Internal server error |

---

## Rate Limiting

Rate limiting is enabled by default to protect against abuse. Each endpoint has specific limits.

### Limits by Endpoint

| Endpoint | Key | Requests | Period | Burst |
|----------|-----|----------|--------|-------|
| `/v1/register` | IP | 5 | 1 min | 2 |
| `/v1/activate` | IP | 5 | 1 min | 2 |
| `/v1/snapshot` | Instance ID | 1 | 1 min | 2 |
| `/api/v1/admin/*` | IP | 30 | 1 min | 10 |
| `/api/v1/healthcheck` | - | unlimited | - | - |

### Response Headers

All rate-limited endpoints return these headers:

| Header | Description |
|--------|-------------|
| `X-RateLimit-Limit` | Maximum requests allowed per period |
| `X-RateLimit-Remaining` | Requests remaining in current period |
| `X-RateLimit-Reset` | Unix timestamp when the limit resets |
| `Retry-After` | Seconds to wait (only on 429 responses) |

### 429 Too Many Requests

When rate limited, the server returns:

```
HTTP/1.1 429 Too Many Requests
X-RateLimit-Limit: 5
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1702847400
Retry-After: 45

Too Many Requests
```

### Brute-Force Protection

Admin endpoints have additional protection: after 5 failed authentication attempts (401/403), the IP is banned for 15 minutes.

### Configuration

All limits are configurable via environment variables. See [README.md](../README.md#rate-limiting) for the full list.

### Recommendations

- **Snapshots**: Every 60 seconds (matches the 1/min limit)
- **Register**: Once per instance lifetime
- **Activate**: Once after registration

---

## Admin API

The following endpoints are intended for administrative use and are not used by instances.

### GET /api/v1/admin/applications

List all applications tracked by the server.

**Response:**

```json
[
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "slug": "my-app",
    "name": "My Awesome App",
    "github_url": "https://github.com/owner/repo",
    "github_stars": 1234,
    "github_stars_updated_at": "2024-01-15T10:30:00Z",
    "logo_url": "https://example.com/logo.png",
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-15T10:30:00Z"
  }
]
```

**Status Codes:**

| Code | Description |
|------|-------------|
| 200 | Success |
| 500 | Server error |

**curl Example:**

```bash
curl https://shm.example.com/api/v1/admin/applications
```

---

### GET /api/v1/admin/applications/{slug}

Get details for a specific application by slug.

**Response:**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "slug": "my-app",
  "name": "My Awesome App",
  "github_url": "https://github.com/owner/repo",
  "github_stars": 1234,
  "github_stars_updated_at": "2024-01-15T10:30:00Z",
  "logo_url": "https://example.com/logo.png",
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-15T10:30:00Z"
}
```

**Status Codes:**

| Code | Description |
|------|-------------|
| 200 | Success |
| 404 | Application not found |
| 500 | Server error |

**curl Example:**

```bash
curl https://shm.example.com/api/v1/admin/applications/my-app
```

---

### PUT /api/v1/admin/applications/{slug}

Update an application's metadata.

**Request Body:**

```json
{
  "github_url": "https://github.com/owner/repo",
  "logo_url": "https://example.com/logo.png"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `github_url` | string | No | GitHub repository URL (must be https://github.com/owner/repo format) |
| `logo_url` | string | No | Custom logo URL |

**Response:**

```json
{
  "status": "ok",
  "message": "Application updated successfully"
}
```

**Status Codes:**

| Code | Description |
|------|-------------|
| 200 | Application updated |
| 400 | Invalid request body or GitHub URL |
| 404 | Application not found |
| 500 | Server error |

**curl Example:**

```bash
curl -X PUT https://shm.example.com/api/v1/admin/applications/my-app \
  -H "Content-Type: application/json" \
  -d '{
    "github_url": "https://github.com/owner/repo",
    "logo_url": "https://example.com/logo.png"
  }'
```

---

### POST /api/v1/admin/applications/{slug}/refresh-stars

Manually trigger a GitHub stars refresh for a specific application.

**Response:**

```json
{
  "status": "ok",
  "stars": 1234,
  "message": "Stars refreshed successfully"
}
```

**Status Codes:**

| Code | Description |
|------|-------------|
| 200 | Stars refreshed successfully |
| 400 | Application has no GitHub URL configured |
| 404 | Application not found |
| 500 | Server error or GitHub API error |

**curl Example:**

```bash
curl -X POST https://shm.example.com/api/v1/admin/applications/my-app/refresh-stars
```

---

## GitHub Stars

SHM automatically fetches and displays GitHub repository stars for applications with a configured `github_url`.

### Automatic Refresh

- Stars are refreshed hourly via a background scheduler
- Only applications with a GitHub URL and stale data (>1 hour old) are refreshed
- Uses 1-hour caching to respect GitHub API rate limits

### Rate Limits

| Authentication | Requests per Hour |
|----------------|-------------------|
| No token (unauthenticated) | 60 |
| With `GITHUB_TOKEN` | 5000 |

**Recommendation:** Set the `GITHUB_TOKEN` environment variable with a GitHub Personal Access Token to avoid rate limit issues.

```bash
export GITHUB_TOKEN="ghp_your_token_here"
```

---

## Public Badge Endpoints

SHM provides public SVG badge endpoints for displaying metrics in README files and documentation. These endpoints require no authentication and have no rate limiting.

All badges return SVG images with:
- **Content-Type:** `image/svg+xml;charset=utf-8`
- **Cache-Control:** `public, max-age=300` (5 minutes)
- **Active instances:** Defined as instances with `last_seen_at` within the last 30 days

### GET /badge/{app-slug}/instances

Returns a badge showing the count of active instances for an application.

**Parameters:**

| Parameter | Location | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `app-slug` | Path | string | Yes | Application slug |
| `color` | Query | string | No | Custom hex color (without #) |
| `label` | Query | string | No | Custom label text (default: "instances") |

**Example:**

```
GET /badge/my-app/instances
GET /badge/my-app/instances?color=00D084&label=deployments
```

**Response:**

SVG image with format: `[label] [count]`

Color-coded based on count:
- Green (#00D084): ≥10 instances
- Yellow (#F59E0B): 5-9 instances
- Red (#EF4444): <5 instances

---

### GET /badge/{app-slug}/version

Returns a badge showing the most commonly used version for an application.

**Parameters:**

| Parameter | Location | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `app-slug` | Path | string | Yes | Application slug |
| `color` | Query | string | No | Custom hex color (without #, default: purple) |
| `label` | Query | string | No | Custom label text (default: "version") |

**Example:**

```
GET /badge/my-app/version
GET /badge/my-app/version?color=8B5CF6&label=latest
```

**Response:**

SVG image with format: `[label] [version]`

If no active instances are found, displays "no data".

---

### GET /badge/{app-slug}/metric/{metric-name}

Returns a badge showing an aggregated metric value across all active instances.

**Parameters:**

| Parameter | Location | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `app-slug` | Path | string | Yes | Application slug |
| `metric-name` | Path | string | Yes | Name of the metric to aggregate |
| `color` | Query | string | No | Custom hex color (without #) |
| `label` | Query | string | No | Custom label text (default: metric name) |

**Example:**

```
GET /badge/my-app/metric/users_count
GET /badge/my-app/metric/documents_count?label=documents
```

**Response:**

SVG image with format: `[label] [value]`

The metric value is summed across all active instances. Numbers are formatted compactly:
- 1000+ → "1.0k"
- 1000000+ → "1.0M"
- 1000000000+ → "1.0B"

Color-coded based on value:
- Green (#00D084): ≥1000
- Yellow (#F59E0B): 100-999
- Red (#EF4444): <100

---

### GET /badge/{app-slug}/combined

Returns a combined badge showing both an aggregated metric value and instance count.

**Parameters:**

| Parameter | Location | Type | Required | Description |
|-----------|----------|------|----------|-------------|
| `app-slug` | Path | string | Yes | Application slug |
| `metric` | Query | string | No | Metric to aggregate (default: "users_count") |
| `color` | Query | string | No | Custom hex color (without #, default: indigo) |
| `label` | Query | string | No | Custom label text (default: "adoption") |

**Example:**

```
GET /badge/my-app/combined
GET /badge/my-app/combined?metric=documents_count&label=usage
```

**Response:**

SVG image with format: `[label] [metric] / [instances]`

Example output: "adoption 1.2k / 42" (1.2k users across 42 instances)

---

### Error Badges

If an error occurs (invalid slug, metric not found, database error), the endpoint returns a red error badge instead of failing:

```
[error] [error message]
```

This ensures badges remain functional even when data is temporarily unavailable.

---

## Data Retention & Instance Lifecycle

### Instance States

Instances go through the following lifecycle:

| State | Description |
|-------|-------------|
| `registered` | Instance has sent its public key but not yet activated |
| `active` | Instance has been activated and is sending snapshots |
| `inactive` | Instance hasn't sent a snapshot in >30 days |

### Active Instance Definition

An instance is considered **active** if its `last_seen_at` timestamp is within the last **30 days**. This affects:
- Badge counts (only active instances are counted)
- Metric aggregations (only active instances contribute)
- Dashboard statistics

### Snapshot Storage

- Each snapshot is stored as a separate row in the database
- Metrics are stored as JSONB for flexible schema
- No automatic cleanup is performed on old snapshots

### Automatic System Metrics

The SDK automatically collects and sends these system metrics alongside your custom metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `cpu_percent` | float | Current CPU usage percentage |
| `memory_mb` | int | Memory usage in megabytes |
| `os` | string | Operating system (linux, darwin, windows) |
| `arch` | string | CPU architecture (amd64, arm64) |
| `go_version` | string | Go runtime version (Go SDK only) |
| `node_version` | string | Node.js version (Node.js SDK only) |

### Data Cleanup

To clean up old data, you can run SQL queries directly:

```sql
-- Delete snapshots older than 90 days
DELETE FROM snapshots WHERE created_at < NOW() - INTERVAL '90 days';

-- Delete inactive instances (no snapshot in 90 days)
DELETE FROM instances WHERE last_seen_at < NOW() - INTERVAL '90 days';
```

> **Note:** Always backup your database before running cleanup queries.

---

## SDKs

Official SDKs are available for easy integration. They handle keypair generation, storage, registration, and periodic snapshot sending automatically.

### Go SDK

```bash
go get github.com/kolapsis/shm/sdk
```

```go
import "github.com/kolapsis/shm/sdk"

telemetry, _ := sdk.New(sdk.Config{
    ServerURL:   "https://shm.example.com",
    AppName:     "my-app",
    AppVersion:  "1.0.0",
    Environment: "production",
    Enabled:     true,
})

telemetry.SetProvider(func() map[string]interface{} {
    return map[string]interface{}{
        "users": getUserCount(),
        "documents": getDocCount(),
    }
})

go telemetry.Start(context.Background())
```

### Node.js / TypeScript SDK

[![npm version](https://img.shields.io/npm/v/@kolapsis/shm-sdk?style=flat-square)](https://www.npmjs.com/package/@kolapsis/shm-sdk)

```bash
npm install @kolapsis/shm-sdk
```

```typescript
import { SHMClient } from '@kolapsis/shm-sdk';

const telemetry = new SHMClient({
    serverUrl: 'https://shm.example.com',
    appName: 'my-app',
    appVersion: '1.0.0',
    environment: 'production',
    enabled: true,
});

telemetry.setProvider(() => ({
    users: getUserCount(),
    documents: getDocCount(),
}));

telemetry.start();
```

> **Note:** Requires Node.js >= 22 LTS. Zero external dependencies.
