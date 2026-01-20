// SPDX-License-Identifier: AGPL-3.0-or-later

import { formatNumber, formatKey, formatResourceKey } from '../utils/formatters.js';
import { getResourceIcon } from '../utils/icons.js';

/**
 * Drawer component - Instance detail panel
 */
export default () => ({
    // Expose utilities to template
    formatNumber,
    formatKey,
    formatResourceKey,
    getResourceIcon,

    // Local state
    confirmDelete: false,
    deleteError: null,

    /**
     * Initialize component and watch for instance changes
     */
    init() {
        this.$watch('$store.dashboard.selectedInstance', () => {
            this.confirmDelete = false;
            this.deleteError = null;
        });
    },

    /**
     * Get the dashboard store
     */
    get store() {
        return this.$store.dashboard;
    },

    /**
     * Get selected instance
     */
    get instance() {
        return this.store.selectedInstance;
    },

    /**
     * Get resource keys
     */
    get resourceKeys() {
        return this.store.currentResourceKeys;
    },

    /**
     * Check if drawer is open
     */
    get isOpen() {
        return this.instance !== null;
    },

    /**
     * Check if deleting
     */
    get isDeleting() {
        return this.store.deletingInstance;
    },

    /**
     * Close the drawer
     */
    close() {
        this.confirmDelete = false;
        this.deleteError = null;
        this.store.closeDrawer();
    },

    /**
     * Show delete confirmation
     */
    showDeleteConfirm() {
        this.confirmDelete = true;
        this.deleteError = null;
    },

    /**
     * Cancel delete
     */
    cancelDelete() {
        this.confirmDelete = false;
        this.deleteError = null;
    },

    /**
     * Delete the instance
     */
    async deleteInstance() {
        this.deleteError = null;
        const success = await this.store.deleteSelectedInstance();
        if (!success) {
            this.deleteError = 'Failed to delete instance. Please try again.';
        }
    }
});
