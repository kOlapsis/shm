// SPDX-License-Identifier: AGPL-3.0-or-later

/**
 * Navbar component - Top navigation bar with instance search
 */
export default () => ({
    searchQuery: '',
    mobileSearchOpen: false,

    init() {
        // Watch for search query changes
        this.$watch('searchQuery', (value) => {
            this.$store.dashboard.instanceSearchQuery = value;
            this.$store.dashboard.handleInstanceSearch();
        });
    },

    toggleMobileSearch() {
        this.mobileSearchOpen = !this.mobileSearchOpen;
        if (this.mobileSearchOpen) {
            // focus the field after the transition opens it
            this.$nextTick(() => {
                this.$refs.mobileSearchInput?.focus();
            });
        }
    },

    /**
     * Get the dashboard store
     */
    get store() {
        return this.$store.dashboard;
    },

    /**
     * Get the current title
     */
    get title() {
        return this.store.selectedApp || 'All Applications';
    },

    /**
     * Get instance count text
     */
    get countText() {
        const count = this.store.selectedApp
            ? this.store.getAppTotalCount(this.store.selectedApp)
            : this.store.stats.total_instances;
        return `(${count} instances)`;
    },

    /**
     * Check if currently searching
     */
    get isSearching() {
        return this.store.searchingInstances;
    }
});
