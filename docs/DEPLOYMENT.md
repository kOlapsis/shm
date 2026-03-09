

# <img src="./logos/shm-logo.svg" width="32"> SHM Deployment Guide

This guide covers deploying SHM (Self-Hosted Metrics) in production.

## Prerequisites

- Docker and Docker Compose
- A domain name (optional but recommended)
- A reverse proxy (Traefik, Nginx, Caddy...)

---

## Quick Start with Docker Compose

### 1. Create the configuration

Create a `compose.yml` file:

```yaml
name: shm

services:
  db:
    image: postgres:15-alpine
    container_name: shm-db
    restart: unless-stopped
    environment:
      POSTGRES_USER: shm
      POSTGRES_PASSWORD: ${DB_PASSWORD:-change-me-in-production}
      POSTGRES_DB: metrics
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./migrations:/docker-entrypoint-initdb.d
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U shm -d metrics"]
      interval: 10s
      timeout: 5s
      retries: 5

  app:
    image: ghcr.io/kolapsis/shm:latest
    # Or build from source:
    # build:
    #   context: .
    #   dockerfile: Dockerfile
    container_name: shm-app
    restart: unless-stopped
    depends_on:
      db:
        condition: service_healthy
    environment:
      SHM_DB_DSN: "postgres://shm:${DB_PASSWORD:-change-me-in-production}@db:5432/metrics?sslmode=disable"
      PORT: "8080"
    ports:
      - "8080:8080"

volumes:
  postgres_data:
```

### 2. Download migrations

```bash
mkdir -p migrations
curl -sL https://raw.githubusercontent.com/kolapsis/shm/main/migrations/001_init.sql -o migrations/001_init.sql
curl -sL https://raw.githubusercontent.com/kolapsis/shm/main/migrations/002_applications.sql -o migrations/002_applications.sql
```

### 3. Start the services

```bash
docker compose up -d
```

The server is now running on port 8080.

---

## Rate Limiting

Rate limiting is enabled by default to protect against abuse.

| Variable | Default | Description |
|----------|---------|-------------|
| `SHM_RATELIMIT_ENABLED` | `true` | Enable/disable rate limiting |
| `SHM_RATELIMIT_CLEANUP_INTERVAL` | `10m` | Interval for cleaning up expired limiters |
| `SHM_RATELIMIT_REGISTER_REQUESTS` | `5` | Max requests per period for `/v1/register` and `/v1/activate` |
| `SHM_RATELIMIT_REGISTER_PERIOD` | `1m` | Time window for register endpoints |
| `SHM_RATELIMIT_REGISTER_BURST` | `2` | Burst allowance for register endpoints |
| `SHM_RATELIMIT_SNAPSHOT_REQUESTS` | `1` | Max requests per period for `/v1/snapshot` (per instance) |
| `SHM_RATELIMIT_SNAPSHOT_PERIOD` | `1m` | Time window for snapshot endpoint |
| `SHM_RATELIMIT_SNAPSHOT_BURST` | `2` | Burst allowance for snapshot endpoint |
| `SHM_RATELIMIT_ADMIN_REQUESTS` | `30` | Max requests per period for `/api/v1/admin/*` |
| `SHM_RATELIMIT_ADMIN_PERIOD` | `1m` | Time window for admin endpoints |
| `SHM_RATELIMIT_ADMIN_BURST` | `10` | Burst allowance for admin endpoints |
| `SHM_RATELIMIT_BRUTEFORCE_THRESHOLD` | `5` | Failed auth attempts before IP ban |
| `SHM_RATELIMIT_BRUTEFORCE_BAN` | `15m` | Duration of IP ban after brute-force detection |

See [API.md](./API.md) for full API and rate limiting documentation.

---

## Security Warning

> **IMPORTANT: The dashboard is NOT secured by default.**
>
> The `/api/v1/admin/*` endpoints and the web dashboard have NO authentication.
> Anyone with network access can view your telemetry data.
>
> **You MUST secure the dashboard before exposing it to the internet.**

The telemetry collection endpoints (`/v1/register`, `/v1/activate`, `/v1/snapshot`) are secured with Ed25519 signatures and can be safely exposed.

---

## Securing the Dashboard

### Option 1: Traefik with ForwardAuth (Recommended)

This is the most flexible approach, supporting SSO providers like Authelia, Authentik, or Keycloak.

**Example with Authelia:**

```yaml
name: shm

services:
  traefik:
    image: traefik:v3.0
    container_name: traefik
    restart: unless-stopped
    command:
      - "--providers.docker=true"
      - "--providers.docker.exposedbydefault=false"
      - "--entrypoints.web.address=:80"
      - "--entrypoints.websecure.address=:443"
      - "--certificatesresolvers.letsencrypt.acme.CHOOSE-YOUR-challenge=true"
      - "--certificatesresolvers.letsencrypt.acme.email=your@email.com"
      - "--certificatesresolvers.letsencrypt.acme.storage=/letsencrypt/acme.json"
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro

  authelia:
    image: authelia/authelia:latest
    container_name: authelia
    restart: unless-stopped
    volumes:
      - ./authelia:/config
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.authelia.rule=Host(`auth.example.com`)"
      - "traefik.http.routers.authelia.entrypoints=websecure"
      - "traefik.http.routers.authelia.tls.certresolver=letsencrypt"

  db:
    image: postgres:15-alpine
    container_name: shm-db
    restart: unless-stopped
    environment:
      POSTGRES_USER: shm
      POSTGRES_PASSWORD: ${DB_PASSWORD}
      POSTGRES_DB: metrics
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U shm -d metrics"]
      interval: 10s
      timeout: 5s
      retries: 5

  app:
    image: ghcr.io/kolapsis/shm:latest
    container_name: shm-app
    restart: unless-stopped
    depends_on:
      db:
        condition: service_healthy
    environment:
      SHM_DB_DSN: "postgres://shm:${DB_PASSWORD}@db:5432/metrics?sslmode=disable"
      PORT: "8080"
    labels:
      - "traefik.enable=true"
      # Public API routes (telemetry collection + healthcheck) - no auth
      - "traefik.http.routers.shm-api.rule=Host(`shm.example.com`) && PathPrefix(`/api/v1/`) && !PathPrefix(`/api/v1/admin/`)"
      - "traefik.http.routers.shm-api.entrypoints=websecure"
      - "traefik.http.routers.shm-api.tls.certresolver=letsencrypt"
      - "traefik.http.routers.shm-api.service=shm"
      - "traefik.http.routers.shm-api.priority=3"
      # Protected admin API - with ForwardAuth
      - "traefik.http.routers.shm-admin.rule=Host(`shm.example.com`) && PathPrefix(`/api/v1/admin/`)"
      - "traefik.http.routers.shm-admin.entrypoints=websecure"
      - "traefik.http.routers.shm-admin.tls.certresolver=letsencrypt"
      - "traefik.http.routers.shm-admin.middlewares=authelia@docker"
      - "traefik.http.routers.shm-admin.service=shm"
      - "traefik.http.routers.shm-admin.priority=2"
      # Protected dashboard (frontend) - with ForwardAuth
      - "traefik.http.routers.shm-dashboard.rule=Host(`shm.example.com`)"
      - "traefik.http.routers.shm-dashboard.entrypoints=websecure"
      - "traefik.http.routers.shm-dashboard.tls.certresolver=letsencrypt"
      - "traefik.http.routers.shm-dashboard.middlewares=authelia@docker"
      - "traefik.http.routers.shm-dashboard.service=shm"
      - "traefik.http.routers.shm-dashboard.priority=1"
      # Service
      - "traefik.http.services.shm.loadbalancer.server.port=8080"
      # ForwardAuth middleware
      - "traefik.http.middlewares.authelia.forwardauth.address=http://authelia:9091/api/authz/forward-auth"
      - "traefik.http.middlewares.authelia.forwardauth.trustForwardHeader=true"
      - "traefik.http.middlewares.authelia.forwardauth.authResponseHeaders=Remote-User,Remote-Groups"

volumes:
  postgres_data:
```

### Option 2: Traefik with Basic Auth

Simpler setup using HTTP Basic Authentication:

```yaml
name: shm

services:
  traefik:
    image: traefik:v3.0
    container_name: traefik
    restart: unless-stopped
    command:
      - "--providers.docker=true"
      - "--providers.docker.exposedbydefault=false"
      - "--entrypoints.web.address=:80"
      - "--entrypoints.websecure.address=:443"
      - "--certificatesresolvers.letsencrypt.acme.tlschallenge=true"
      - "--certificatesresolvers.letsencrypt.acme.email=your@email.com"
      - "--certificatesresolvers.letsencrypt.acme.storage=/letsencrypt/acme.json"
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - letsencrypt:/letsencrypt

  db:
    image: postgres:15-alpine
    container_name: shm-db
    restart: unless-stopped
    environment:
      POSTGRES_USER: shm
      POSTGRES_PASSWORD: ${DB_PASSWORD}
      POSTGRES_DB: metrics
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U shm -d metrics"]
      interval: 10s
      timeout: 5s
      retries: 5

  app:
    image: ghcr.io/kolapsis/shm:latest
    container_name: shm-app
    restart: unless-stopped
    depends_on:
      db:
        condition: service_healthy
    environment:
      SHM_DB_DSN: "postgres://shm:${DB_PASSWORD}@db:5432/metrics?sslmode=disable"
      PORT: "8080"
    labels:
      - "traefik.enable=true"
      # Public API routes (telemetry collection + healthcheck) - no auth
      - "traefik.http.routers.shm-api.rule=Host(`shm.example.com`) && PathPrefix(`/api/v1/`) && !PathPrefix(`/api/v1/admin/`)"
      - "traefik.http.routers.shm-api.entrypoints=websecure"
      - "traefik.http.routers.shm-api.tls.certresolver=letsencrypt"
      - "traefik.http.routers.shm-api.service=shm"
      - "traefik.http.routers.shm-api.priority=3"
      # Protected admin API - with Basic Auth
      - "traefik.http.routers.shm-admin.rule=Host(`shm.example.com`) && PathPrefix(`/api/v1/admin/`)"
      - "traefik.http.routers.shm-admin.entrypoints=websecure"
      - "traefik.http.routers.shm-admin.tls.certresolver=letsencrypt"
      - "traefik.http.routers.shm-admin.middlewares=shm-auth"
      - "traefik.http.routers.shm-admin.service=shm"
      - "traefik.http.routers.shm-admin.priority=2"
      # Protected dashboard (frontend) - with Basic Auth
      - "traefik.http.routers.shm-dashboard.rule=Host(`shm.example.com`)"
      - "traefik.http.routers.shm-dashboard.entrypoints=websecure"
      - "traefik.http.routers.shm-dashboard.tls.certresolver=letsencrypt"
      - "traefik.http.routers.shm-dashboard.middlewares=shm-auth"
      - "traefik.http.routers.shm-dashboard.service=shm"
      - "traefik.http.routers.shm-dashboard.priority=1"
      # Service
      - "traefik.http.services.shm.loadbalancer.server.port=8080"
      # Basic Auth middleware (generate with: htpasswd -nb admin password)
      - "traefik.http.middlewares.shm-auth.basicauth.users=${BASIC_AUTH_USERS}"

volumes:
  postgres_data:
  letsencrypt:
```

Generate the Basic Auth credentials:

```bash
# Install htpasswd (apache2-utils on Debian/Ubuntu)
htpasswd -nb admin your-secure-password
# Output: admin:$$apr1$$xyz...

# Add to .env file (escape $ with $$)
echo 'BASIC_AUTH_USERS=admin:$$apr1$$xyz...' >> .env
```

### Option 3: Nginx with Basic Auth

```nginx
upstream shm {
    server 127.0.0.1:8080;
}

server {
    listen 443 ssl http2;
    server_name shm.example.com;

    ssl_certificate /etc/letsencrypt/live/shm.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/shm.example.com/privkey.pem;

    # Public API (telemetry + healthcheck) - no auth
    location /api/v1/ {
        # Exclude admin endpoints
        location /api/v1/admin/ {
            auth_basic "SHM Admin";
            auth_basic_user_file /etc/nginx/.htpasswd;

            proxy_pass http://shm;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
        }

        proxy_pass http://shm;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # Protected dashboard (frontend)
    location / {
        auth_basic "SHM Dashboard";
        auth_basic_user_file /etc/nginx/.htpasswd;

        proxy_pass http://shm;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Create the password file:

```bash
htpasswd -c /etc/nginx/.htpasswd admin
```

### Option 4: Caddy with Basic Auth

```caddyfile
shm.example.com {
    # Public API (telemetry + healthcheck) - no auth
    @public_api {
        path /api/v1/*
        not path /api/v1/admin/*
    }
    handle @public_api {
        reverse_proxy localhost:8080
    }

    # Protected admin API and dashboard
    handle {
        basicauth {
            admin $2a$14$... # bcrypt hash
        }
        reverse_proxy localhost:8080
    }
}
```

Generate bcrypt hash:

```bash
caddy hash-password --plaintext 'your-secure-password'
```

---

## Environment Variables

### Server-side

| Variable | Default | Description |
|----------|---------|-------------|
| `SHM_DB_DSN` | (required) | PostgreSQL connection string |
| `PORT` | `8080` | HTTP server port |
| `GITHUB_TOKEN` | - | GitHub Personal Access Token for higher API rate limits |

For the full list of environment variables (including rate limiting), see [README.md](../README.md#environment-variables).

### Client-side (SDK)

These environment variables are read by the SHM clients (Go, Node.js) running in your applications:

| Variable | Default | Description |
|----------|---------|-------------|
| `DO_NOT_TRACK` | - | Set to `true` or `1` to **completely disable telemetry**. No data will be sent to the server. This overrides the `enabled` configuration option. |
| `SHM_COLLECT_SYSTEM_METRICS` | `true` | Set to `false` or `0` to disable automatic system metrics collection (OS, CPU, memory). Custom metrics will still be sent. |

#### Respecting User Privacy

All SHM clients respect the standard `DO_NOT_TRACK` environment variable. This allows end-users to opt-out of telemetry at the system level:

```bash
# In the environment where your application runs
export DO_NOT_TRACK=true
```

When `DO_NOT_TRACK` is enabled:
- No network requests are made to the SHM server
- No identity file is accessed
- The client silently disables itself

This is useful for:
- Users who want to opt-out of all telemetry
- Development/testing environments
- Privacy-conscious deployments

---

## Database Backups

### Backup

```bash
docker exec shm-db pg_dump -U shm metrics > backup_$(date +%Y%m%d).sql
```

### Restore

```bash
cat backup_20240115.sql | docker exec -i shm-db psql -U shm metrics
```

---

## Health Checks

The application exposes a dedicated healthcheck endpoint:

```bash
curl -f http://localhost:8080/api/v1/healthcheck || exit 1
# Returns: {"status":"ok"}
```

For Kubernetes or orchestrators:

```yaml
livenessProbe:
  httpGet:
    path: /api/v1/healthcheck
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10
readinessProbe:
  httpGet:
    path: /api/v1/healthcheck
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
```

The `/api/v1/healthcheck` endpoint has no rate limiting and no authentication.

---

## Resource Requirements

Minimal requirements:

- **CPU**: 0.1 vCPU
- **Memory**: 64 MB (app) + 256 MB (PostgreSQL)
- **Disk**: Depends on data retention, ~1 KB per snapshot

---

## Upgrading

```bash
# Pull latest image
docker compose pull

# Restart with new version
docker compose up -d
```

Database migrations are applied automatically on startup.

---

## Troubleshooting

### Check logs

```bash
# All services
docker compose logs -f

# App only
docker compose logs -f app

# Database only
docker compose logs -f db
```

### Database connection issues

```bash
# Test database connectivity
docker exec shm-app sh -c 'nc -zv db 5432'

# Check database status
docker exec shm-db pg_isready -U shm -d metrics
```

### Reset everything

```bash
docker compose down -v  # Warning: deletes all data!
docker compose up -d
```
