// SPDX-License-Identifier: AGPL-3.0-or-later

const API_BASE = '/api/v1/admin';

export async function fetchStats() {
    const response = await fetch(`${API_BASE}/stats`);
    if (!response.ok) throw new Error('Failed to fetch stats');
    return response.json();
}

export async function fetchApplications() {
    const response = await fetch(`${API_BASE}/applications`);
    if (!response.ok) throw new Error('Failed to fetch applications');
    return response.json();
}

export async function fetchInstances({ offset = 0, limit = 50, app = null, query = null } = {}) {
    const params = new URLSearchParams();
    params.set('offset', offset);
    params.set('limit', limit);

    if (app) params.set('app', app);
    if (query?.trim()) params.set('q', query.trim());

    const response = await fetch(`${API_BASE}/instances?${params.toString()}`);
    if (!response.ok) throw new Error('Failed to fetch instances');
    return response.json();
}

export async function fetchMetrics(appName, period = '24h') {
    const response = await fetch(
        `${API_BASE}/metrics/${encodeURIComponent(appName)}?period=${period}`
    );
    if (!response.ok) throw new Error('Failed to fetch metrics');
    return response.json();
}

export async function updateApplication(slug, data) {
    const response = await fetch(`${API_BASE}/applications/${encodeURIComponent(slug)}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data)
    });
    if (!response.ok) {
        const error = await response.text();
        throw new Error(error || 'Failed to update application');
    }
    return response.json();
}

export async function refreshApplicationStars(slug) {
    const response = await fetch(`${API_BASE}/applications/${encodeURIComponent(slug)}/refresh-stars`, {
        method: 'POST'
    });
    if (!response.ok) {
        const error = await response.text();
        throw new Error(error || 'Failed to refresh stars');
    }
    return response.json();
}

export async function deleteInstance(instanceId) {
    const response = await fetch(`${API_BASE}/instances/${encodeURIComponent(instanceId)}`, {
        method: 'DELETE'
    });
    if (!response.ok) {
        const error = await response.text();
        throw new Error(error || 'Failed to delete instance');
    }
    return response.json();
}
