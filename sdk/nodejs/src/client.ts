// SPDX-License-Identifier: MIT

import { existsSync, readFileSync } from 'node:fs';
import { arch, platform, cpus } from 'node:os';
import { memoryUsage } from 'node:process';
import { join } from 'node:path';
import type {
  Config,
  Identity,
  MetricsProvider,
  RegisterRequest,
  SnapshotRequest,
  SystemMetrics,
} from './types.js';
import { loadOrGenerateIdentity } from './identity.js';
import { signMessage } from './crypto.js';

const DEFAULT_REPORT_INTERVAL_MS = 3600000; // 1 hour
const MIN_REPORT_INTERVAL_MS = 60000; // 1 minute
const HTTP_TIMEOUT_MS = 10000; // 10 seconds

/**
 * SHM Client for privacy-first telemetry.
 *
 * @example
 * ```typescript
 * import { SHMClient } from '@shm/nodejs-sdk';
 *
 * const client = new SHMClient({
 *   serverUrl: 'https://telemetry.example.com',
 *   appName: 'my-app',
 *   appVersion: '1.0.0',
 *   environment: 'production',
 * });
 *
 * // Optional: add custom metrics
 * client.setProvider(() => ({
 *   users_count: getUserCount(),
 *   jobs_processed: getJobsCount(),
 * }));
 *
 * // Start telemetry (runs in background)
 * const controller = client.start();
 *
 * // To stop later:
 * controller.abort();
 * ```
 */
export class SHMClient {
  private readonly config: Required<Omit<Config, 'environment'>> & { environment?: string };
  private readonly identity: Identity;
  private provider: MetricsProvider | null = null;
  private readonly startTime: Date;
  private intervalId: ReturnType<typeof setInterval> | null = null;
  private abortController: AbortController | null = null;

  /**
   * Creates a new SHM client instance.
   *
   * @param config - Client configuration.
   * @throws Error if identity cannot be initialized.
   */
  constructor(config: Config) {
    const dataDir = config.dataDir || '.';
    let reportIntervalMs = config.reportIntervalMs || DEFAULT_REPORT_INTERVAL_MS;

    if (reportIntervalMs < MIN_REPORT_INTERVAL_MS) {
      reportIntervalMs = MIN_REPORT_INTERVAL_MS;
    }

    this.config = {
      serverUrl: config.serverUrl,
      appName: config.appName,
      appVersion: config.appVersion,
      dataDir,
      environment: config.environment,
      enabled: isDoNotTrack() ? false : (config.enabled ?? true),
      reportIntervalMs,
      collectSystemMetrics: config.collectSystemMetrics ?? collectSystemMetricsFromEnv(),
    };

    if (this.config.enabled) {
      // Load or generate identity
      const idPath = join(dataDir, `shm_identity.json`);
      this.identity = loadOrGenerateIdentity(idPath);
    } else {
      this.identity = { instanceId: '', privateKey: '', publicKey: '' };
    }
    this.startTime = new Date();
  }

  /**
   * Sets a custom metrics provider function.
   * The provider is called before each snapshot to collect custom metrics.
   *
   * @param provider - Function returning custom metrics object.
   */
  setProvider(provider: MetricsProvider): void {
    this.provider = provider;
  }

  /**
   * Starts the telemetry client.
   * Registers the instance, activates it, and begins periodic snapshot reporting.
   *
   * @returns AbortController to stop the client.
   */
  start(): AbortController {
    this.abortController = new AbortController();

    if (!this.config.enabled) {
      this.log('Telemetry disabled');
      return this.abortController;
    }

    // Start async operations
    this.startAsync(this.abortController.signal).catch((err) => {
      this.log(`Start error: ${err}`);
    });

    return this.abortController;
  }

  /**
   * Stops the telemetry client.
   */
  stop(): void {
    if (this.intervalId) {
      clearInterval(this.intervalId);
      this.intervalId = null;
    }
    if (this.abortController) {
      this.abortController.abort();
      this.abortController = null;
    }
  }

  private async startAsync(signal: AbortSignal): Promise<void> {
    // Register instance
    try {
      await this.register(signal);
    } catch (err) {
      this.log(`Register warning: ${err}`);
    }

    if (signal.aborted) return;

    // Activate instance
    try {
      await this.activate(signal);
    } catch (err) {
      this.log(`Activation failed: ${err}`);
    }

    if (signal.aborted) return;

    // Send initial snapshot
    await this.sendSnapshot(signal);

    if (signal.aborted) return;

    // Start periodic snapshots
    this.intervalId = setInterval(() => {
      if (!signal.aborted) {
        this.sendSnapshot(signal).catch((err) => {
          this.log(`Snapshot error: ${err}`);
        });
      }
    }, this.config.reportIntervalMs);

    // Clear interval on abort
    signal.addEventListener('abort', () => {
      if (this.intervalId) {
        clearInterval(this.intervalId);
        this.intervalId = null;
      }
    });
  }

  private async register(signal: AbortSignal): Promise<void> {
    const payload: RegisterRequest = {
      instance_id: this.identity.instanceId,
      public_key: this.identity.publicKey,
      app_name: this.config.appName,
      app_version: this.config.appVersion,
      environment: this.config.environment,
      os_arch: `${platform()}/${arch()}`,
    };

    const response = await this.fetch('/v1/register', {
      method: 'POST',
      body: JSON.stringify(payload),
      signal,
    });

    if (response.status !== 200 && response.status !== 201) {
      throw new Error(`server returned ${response.status}`);
    }

    this.log(`Instance registered: ${this.identity.instanceId}`);
  }

  private async activate(signal: AbortSignal): Promise<void> {
    const payload = { action: 'activate' };
    const body = JSON.stringify(payload);

    const signature = signMessage(this.identity.privateKey, body);

    const response = await this.fetch('/v1/activate', {
      method: 'POST',
      body,
      signal,
      headers: {
        'X-Instance-ID': this.identity.instanceId,
        'X-Signature': signature,
      },
    });

    if (response.status !== 200) {
      throw new Error(`activation failed: code ${response.status}`);
    }

    this.log('Instance ACTIVATED successfully');
  }

  private async sendSnapshot(signal: AbortSignal): Promise<void> {
    try {
      // Collect metrics
      let customMetrics: Record<string, unknown> = {};
      if (this.provider) {
        customMetrics = await this.provider();
      }

      const systemMetrics = this.config.collectSystemMetrics ? this.getSystemMetrics() : {};
      const metrics = { ...customMetrics, ...systemMetrics };

      const payload: SnapshotRequest = {
        instance_id: this.identity.instanceId,
        timestamp: new Date().toISOString(),
        metrics,
      };

      const body = JSON.stringify(payload);
      const signature = signMessage(this.identity.privateKey, body);

      const response = await this.fetch('/v1/snapshot', {
        method: 'POST',
        body,
        signal,
        headers: {
          'X-Instance-ID': this.identity.instanceId,
          'X-Signature': signature,
        },
      });

      if (response.status !== 202) {
        this.log(`Snapshot rejected: ${response.status}`);
      } else {
        this.log('Snapshot sent successfully');
      }
    } catch (err) {
      if (!signal.aborted) {
        this.log(`Failed to send snapshot: ${err}`);
      }
    }
  }

  private getSystemMetrics(): SystemMetrics {
    const mem = memoryUsage();
    const uptimeHours = Math.floor((Date.now() - this.startTime.getTime()) / 3600000);

    const metrics: SystemMetrics = {
      sys_os: platform(),
      sys_arch: arch(),
      sys_cpu_cores: cpus().length,
      sys_node_version: process.version,
      sys_mode: detectDeploymentMode(),
      app_mem_heap_mb: Math.floor(mem.heapUsed / 1024 / 1024),
      app_mem_rss_mb: Math.floor(mem.rss / 1024 / 1024),
    };

    if (uptimeHours > 0) {
      metrics.app_uptime_h = uptimeHours;
    }

    return metrics;
  }

  private async fetch(
    path: string,
    options: {
      method: string;
      body?: string;
      signal?: AbortSignal;
      headers?: Record<string, string>;
    }
  ): Promise<Response> {
    const url = `${this.config.serverUrl}${path}`;

    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), HTTP_TIMEOUT_MS);

    // Combine signals
    const signal = options.signal;
    if (signal) {
      signal.addEventListener('abort', () => controller.abort());
    }

    try {
      const response = await fetch(url, {
        method: options.method,
        body: options.body,
        signal: controller.signal,
        headers: {
          'Content-Type': 'application/json',
          ...options.headers,
        },
      });
      return response;
    } finally {
      clearTimeout(timeoutId);
    }
  }

  private log(message: string): void {
    console.log(`[SHM] ${message}`);
  }
}

/**
 * Detects the deployment mode (kubernetes, docker, or standalone).
 */
function detectDeploymentMode(): string {
  // Check for Kubernetes
  if (process.env['KUBERNETES_SERVICE_HOST'] && process.env['KUBERNETES_PORT']) {
    return 'kubernetes';
  }

  // Check for Kubernetes service account
  if (existsSync('/var/run/secrets/kubernetes.io/serviceaccount')) {
    return 'kubernetes';
  }

  // Check for Docker
  if (existsSync('/.dockerenv')) {
    return 'docker';
  }

  // Check cgroup for container indicators
  if (isContainerCgroup()) {
    return 'docker';
  }

  return 'standalone';
}

/**
 * Checks if running inside a container based on cgroup info.
 */
function isContainerCgroup(): boolean {
  try {
    const cgroupPath = '/proc/self/cgroup';
    if (!existsSync(cgroupPath)) {
      return false;
    }

    const content = readFileSync(cgroupPath, 'utf-8');
    return (
      content.includes('docker') ||
      content.includes('lxc') ||
      content.includes('containerd')
    );
  } catch {
    return false;
  }
}

/**
 * Reads SHM_COLLECT_SYSTEM_METRICS environment variable.
 * Returns true by default (enabled), false only if explicitly set to "false" or "0".
 */
export function collectSystemMetricsFromEnv(): boolean {
  const val = (process.env['SHM_COLLECT_SYSTEM_METRICS'] ?? '').toLowerCase();
  return val !== 'false' && val !== '0';
}

/**
 * Checks if DO_NOT_TRACK environment variable is set to disable telemetry.
 */
function isDoNotTrack(): boolean {
  const val = (process.env['DO_NOT_TRACK'] ?? '').toLowerCase();
  return val === 'true' || val === '1';
}
