// SPDX-License-Identifier: AGPL-3.0-or-later

import { fetchMetrics } from '../utils/api.js';
import { formatNumber, formatKey, canvasId } from '../utils/formatters.js';

/**
 * Charts store - manages Chart.js instances and chart data
 */
export default {
    // Available periods
    PERIODS: ['24h', '7d', '30d', '3m', '1y', 'all'],

    // State per app
    selectedMetric: {},    // appName -> metricKey
    chartPeriods: {},      // appName -> period
    chartsLoading: {},     // appName -> boolean
    chartsNoData: {},      // appName -> boolean
    chartDataCache: {},    // "appName-period" -> data
    chartInstances: {},    // appName -> Chart instance
    chartRenderTokens: {}, // appName -> token (for render invalidation)
    renderRetryTimers: {}, // appName -> timer

    /**
     * Toggle metric chart visibility
     */
    async toggleMetric(appName, metricKey) {
        if (this.selectedMetric[appName] === metricKey) {
            this.closeChart(appName);
            return;
        }

        // Destroy old chart if switching
        this.destroyChart(appName);
        this.invalidateRender(appName);

        this.selectedMetric[appName] = metricKey;
        this.chartsNoData[appName] = false;

        await this.loadChartData(appName, metricKey);
    },

    /**
     * Close chart for an app
     */
    closeChart(appName) {
        this.destroyChart(appName);
        this.selectedMetric[appName] = null;
        this.chartsNoData[appName] = false;
        this.invalidateRender(appName);
    },

    /**
     * Destroy Chart.js instance
     */
    destroyChart(appName) {
        if (this.chartInstances[appName]) {
            this.chartInstances[appName].destroy();
            delete this.chartInstances[appName];
        }
    },

    /**
     * Select a new period for an app's chart
     */
    async selectPeriod(appName, period) {
        this.chartPeriods[appName] = period;
        const metricKey = this.selectedMetric[appName];

        if (metricKey) {
            delete this.chartDataCache[`${appName}-${period}`];
            this.chartsNoData[appName] = false;
            this.invalidateRender(appName);
            await this.loadChartData(appName, metricKey);
        }
    },

    /**
     * Get current period for an app
     */
    getPeriod(appName) {
        return this.chartPeriods[appName] || '24h';
    },

    /**
     * Load chart data from API
     */
    async loadChartData(appName, metricKey) {
        this.chartsLoading[appName] = true;
        this.chartsNoData[appName] = false;

        try {
            const period = this.getPeriod(appName);
            const cacheKey = `${appName}-${period}`;

            if (!this.chartDataCache[cacheKey]) {
                this.chartDataCache[cacheKey] = await fetchMetrics(appName, period);
            }

            this.chartsLoading[appName] = false;
            const token = this.chartRenderTokens[appName] || 0;
            this.renderChart(appName, metricKey, this.chartDataCache[cacheKey], 0, token);
        } catch (e) {
            console.error('Failed to load chart data:', e);
            this.chartsLoading[appName] = false;
        }
    },

    /**
     * Render chart with retry logic for DOM readiness
     */
    renderChart(appName, metricKey, data, attempt = 0, token = 0) {
        const id = canvasId(appName);

        requestAnimationFrame(() => {
            requestAnimationFrame(() => {
                // Abort if render token is stale or metric changed
                if ((this.chartRenderTokens[appName] || 0) !== token) return;
                if (this.selectedMetric[appName] !== metricKey) return;

                const canvas = document.getElementById(id);
                if (!canvas || !(canvas instanceof HTMLCanvasElement) || !canvas.isConnected) {
                    if (attempt < 10) {
                        this.scheduleRetry(appName, () =>
                            this.renderChart(appName, metricKey, data, attempt + 1, token), 100
                        );
                    }
                    return;
                }

                const rect = canvas.getBoundingClientRect();
                if (rect.width <= 0 && attempt < 10) {
                    this.scheduleRetry(appName, () =>
                        this.renderChart(appName, metricKey, data, attempt + 1, token), 50
                    );
                    return;
                }

                this.createChart(appName, canvas, metricKey, data);
            });
        });
    },

    /**
     * Create the Chart.js instance
     */
    createChart(appName, canvas, metricKey, data) {
        this.destroyChart(appName);

        const timestamps = data.timestamps || [];
        const values = data.metrics?.[metricKey] || [];

        if (!values.length) {
            this.chartsNoData[appName] = true;
            return;
        }

        try {
            const ctx = canvas.getContext('2d');
            if (!ctx) return;

            // Map (timestamp, value) into Chart.js {x, y} points so the time scale
            // can drive tick density itself — no per-point labels in the DOM.
            const points = timestamps.map((ts, i) => ({ x: new Date(ts).getTime(), y: values[i] }));
            const period = this.getPeriod(appName);
            const timeUnit = this.timeUnitFor(period);

            this.chartInstances[appName] = new Chart(ctx, {
                type: 'line',
                data: {
                    datasets: [{
                        label: formatKey(metricKey),
                        data: points,
                        borderColor: '#6366f1',
                        backgroundColor: 'rgba(99, 102, 241, 0.1)',
                        borderWidth: 2,
                        fill: true,
                        tension: 0.3,
                        pointRadius: 0,
                        pointHoverRadius: 4,
                        pointBackgroundColor: '#6366f1',
                        pointBorderColor: '#1a202c',
                        pointBorderWidth: 2
                    }]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: true,
                    aspectRatio: 3,
                    animation: false,
                    interaction: { mode: 'index', intersect: false },
                    plugins: {
                        legend: { display: false },
                        tooltip: {
                            backgroundColor: 'rgba(17, 24, 39, 0.95)',
                            titleColor: '#f3f4f6',
                            bodyColor: '#d1d5db',
                            borderColor: '#374151',
                            borderWidth: 1,
                            padding: 12,
                            displayColors: false,
                            callbacks: { label: ctx => formatNumber(ctx.parsed.y) }
                        }
                    },
                    scales: {
                        x: {
                            type: 'time',
                            time: {
                                unit: timeUnit,
                                tooltipFormat: 'PPpp',
                                displayFormats: {
                                    minute: 'HH:mm',
                                    hour: 'MMM d HH:mm',
                                    day: 'MMM d',
                                    week: 'MMM d',
                                    month: 'MMM yyyy'
                                }
                            },
                            grid: { color: '#1f2937', drawBorder: false },
                            ticks: {
                                color: '#6b7280',
                                font: { size: 10 },
                                maxRotation: 0,
                                autoSkip: true,
                                autoSkipPadding: 24,
                                maxTicksLimit: 8
                            }
                        },
                        y: {
                            beginAtZero: true,
                            grid: { color: '#1f2937', drawBorder: false },
                            ticks: { color: '#6b7280', font: { size: 10 }, callback: v => formatNumber(v) }
                        }
                    }
                }
            });

            requestAnimationFrame(() => {
                this.chartInstances[appName]?.resize();
            });
        } catch (err) {
            console.error('Chart render failed:', err);
            this.destroyChart(appName);
        }
    },

    /**
     * Pick the Chart.js time unit that matches the server bucket size for a period.
     */
    timeUnitFor(period) {
        switch (period) {
            case '7d':  return 'hour';
            case '30d': return 'day';
            case '3m':  return 'day';
            case '1y':  return 'week';
            case 'all': return 'week';
            default:    return 'hour'; // 24h
        }
    },

    /**
     * Schedule a render retry
     */
    scheduleRetry(appName, fn, delay) {
        clearTimeout(this.renderRetryTimers[appName]);
        this.renderRetryTimers[appName] = setTimeout(fn, delay);
    },

    /**
     * Invalidate pending renders
     */
    invalidateRender(appName) {
        this.chartRenderTokens[appName] = (this.chartRenderTokens[appName] || 0) + 1;
        clearTimeout(this.renderRetryTimers[appName]);
        delete this.renderRetryTimers[appName];
    },

    /**
     * Cleanup all charts
     */
    destroy() {
        Object.keys(this.chartInstances).forEach(appName => {
            this.destroyChart(appName);
        });
        Object.values(this.renderRetryTimers).forEach(timer => clearTimeout(timer));
    }
};
