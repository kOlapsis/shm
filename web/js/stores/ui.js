// SPDX-License-Identifier: AGPL-3.0-or-later

const MOBILE_BREAKPOINT = 1024;

export default {
    mobileMenuOpen: false,

    init() {
        const onResize = () => {
            if (window.innerWidth >= MOBILE_BREAKPOINT && this.mobileMenuOpen) {
                this.mobileMenuOpen = false;
            }
        };
        window.addEventListener('resize', onResize, { passive: true });
        window.addEventListener('keydown', (e) => {
            if (e.key === 'Escape' && this.mobileMenuOpen) {
                this.mobileMenuOpen = false;
            }
        });
    },

    openMenu() { this.mobileMenuOpen = true; },
    closeMenu() { this.mobileMenuOpen = false; },
    toggleMenu() { this.mobileMenuOpen = !this.mobileMenuOpen; },

    isMobile() {
        return window.innerWidth < MOBILE_BREAKPOINT;
    }
};
