// SPDX-License-Identifier: AGPL-3.0-or-later

/**
 * Sidebar component - Application navigation
 */
export default () => ({
    /**
     * Get the dashboard store
     */
    get store() {
        return this.$store.dashboard;
    },

    /**
     * Select an application
     */
    selectApp(appName) {
        this.store.selectApp(appName);
        if (this.$store.ui?.isMobile()) {
            this.$store.ui.closeMenu();
        }
    },

    /**
     * Handle app search input
     */
    onSearch(value) {
        this.store.searchQuery = value;
    },

    /**
     * Refresh all data
     */
    refresh() {
        this.store.fetchInitialData();
    },

    /**
     * Get filtered applications
     */
    get filteredApps() {
        return this.store.filteredApplications;
    },

    /**
     * Check if an app is selected
     */
    isSelected(appName) {
        return this.store.selectedApp === appName;
    },

    /**
     * Get instance count for an app
     */
    getCount(appName) {
        return this.store.getAppTotalCount(appName);
    }
});
