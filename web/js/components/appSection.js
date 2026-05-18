// SPDX-License-Identifier: AGPL-3.0-or-later

import { formatNumber, formatKey, timeAgo, canvasId, healthMeta } from '../utils/formatters.js';
import { getOSIcon, getIconForTag } from '../utils/icons.js';

/**
 * App Section component - Displays a single application's data
 */
export default () => ({
    // Expose utilities to template
    formatNumber,
    formatKey,
    timeAgo,
    canvasId,
    getOSIcon,
    getIconForTag,
    healthMeta,

    /**
     * Get the dashboard store
     */
    get dashboard() {
        return this.$store.dashboard;
    },

    /**
     * Get the charts store
     */
    get charts() {
        return this.$store.charts;
    },

    /**
     * Get instance count for an app
     */
    getAppTotalCount(appName) {
        return this.dashboard.getAppTotalCount(appName);
    },

    /**
     * Toggle metric chart
     */
    toggleMetricChart(appName, metricKey) {
        this.charts.toggleMetric(appName, metricKey);
    },

    /**
     * Select chart period
     */
    selectPeriod(appName, period) {
        this.charts.selectPeriod(appName, period);
    },

    /**
     * Close chart
     */
    closeChart(appName) {
        this.charts.closeChart(appName);
    },

    /**
     * Open instance drawer
     */
    openDrawer(instance, resourceKeys) {
        this.dashboard.openDrawer(instance, resourceKeys);
    },

    /**
     * Open edit modal for an application
     */
    openEditModal(appSlug) {
        this.dashboard.openEditModal(appSlug);
    },

    /**
     * Check if a metric is selected for an app
     */
    isMetricSelected(appName, metricKey) {
        return this.charts.selectedMetric[appName] === metricKey;
    },

    /**
     * Get selected metric for an app
     */
    getSelectedMetric(appName) {
        return this.charts.selectedMetric[appName];
    },

    /**
     * Get current period for an app
     */
    getPeriod(appName) {
        return this.charts.getPeriod(appName);
    },

    /**
     * Check if chart is loading
     */
    isChartLoading(appName) {
        return this.charts.chartsLoading[appName];
    },

    /**
     * Check if chart has no data
     */
    hasNoData(appName) {
        return this.charts.chartsNoData[appName];
    },

    /**
     * Get available periods
     */
    get periods() {
        return this.charts.PERIODS;
    }
});
