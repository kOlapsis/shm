// SPDX-License-Identifier: AGPL-3.0-or-later

/**
 * Formatting utilities for the SHM dashboard
 */

/**
 * Format a number with k/M suffixes for readability
 * @param {number|null|undefined} num - The number to format
 * @returns {string} Formatted number string
 */
export function formatNumber(num) {
    if (num == null) return '-';
    if (num >= 1e6) return (num / 1e6).toFixed(1) + 'M';
    if (num >= 1e3) return (num / 1e3).toFixed(1) + 'k';
    return String(num);
}

/**
 * Format a metric key for display (remove _total suffix, replace underscores)
 * @param {string} key - The metric key
 * @returns {string} Formatted key
 */
export function formatKey(key) {
    if (key === undefined || key === null) return '';
    return String(key).replace(/_total$/, '').replace(/_/g, ' ').trim();
}

/**
 * Format a resource key (remove sys_/app_ prefix)
 * @param {string} key - The resource key
 * @returns {string} Formatted key
 */
export function formatResourceKey(key) {
    if (key === undefined || key === null) return '';
    return String(key).replace(/^(sys_|app_)/, '').replace(/_/g, ' ');
}

/**
 * Format a date as relative time ago
 * @param {string|Date|null} dateInput - The date to format
 * @returns {string} Relative time string
 */
export function timeAgo(dateInput) {
    if (!dateInput) return '-';
    const seconds = Math.floor((Date.now() - new Date(dateInput)) / 1000);
    if (seconds < 60) return 'Just now';
    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) return `${minutes}m ago`;
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return `${hours}h ago`;
    return `${Math.floor(hours / 24)}d ago`;
}

/**
 * Generate a simple hash from a string (for unique IDs)
 * @param {string} str - Input string
 * @returns {string} Hash string in base36
 */
export function hash(str) {
    let h = 0;
    const s = String(str);
    for (let i = 0; i < s.length; i++) {
        h = Math.imul(31, h) + s.charCodeAt(i) | 0;
    }
    return Math.abs(h).toString(36);
}

/**
 * Create a URL-safe slug from a string
 * @param {string} str - Input string
 * @returns {string} Slugified string
 */
export function slug(str) {
    return String(str)
        .toLowerCase()
        .trim()
        .replace(/[^a-z0-9_-]+/g, '-')
        .replace(/^-+|-+$/g, '');
}

/**
 * Generate a unique canvas ID for a given app name
 * @param {string} appName - Application name
 * @returns {string} Unique canvas ID
 */
export function canvasId(appName) {
    const s = slug(appName);
    const short = hash(appName).slice(0, 6);
    return `chart-${s}-${short}`;
}

/**
 * Visual presentation for an instance health value (see domain.InstanceHealth).
 * Inactive/abandoned are filtered out server-side so they get a neutral fallback.
 * @param {string} health
 * @returns {{label: string, dot: string, badge: string}}
 */
export function healthMeta(health) {
    switch (health) {
        case 'ok':
            return {
                label: 'OK',
                dot: 'bg-emerald-500 shadow-[0_0_5px_rgba(16,185,129,0.5)]',
                badge: 'bg-emerald-500/10 text-emerald-500 border-emerald-500/20',
            };
        case 'late':
            return {
                label: 'Late',
                dot: 'bg-amber-500 shadow-[0_0_5px_rgba(245,158,11,0.5)]',
                badge: 'bg-amber-500/10 text-amber-500 border-amber-500/20',
            };
        case 'silent':
            return {
                label: 'Silent',
                dot: 'bg-red-500 shadow-[0_0_5px_rgba(239,68,68,0.5)]',
                badge: 'bg-red-500/10 text-red-500 border-red-500/20',
            };
        default:
            return {
                label: health || 'unknown',
                dot: 'bg-gray-500',
                badge: 'bg-gray-500/10 text-gray-400 border-gray-500/20',
            };
    }
}
