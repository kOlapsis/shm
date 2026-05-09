// SPDX-License-Identifier: AGPL-3.0-or-later

import Alpine from 'https://cdn.jsdelivr.net/npm/alpinejs@3.14.3/dist/module.esm.js';

import dashboardStore from './stores/dashboard.js';
import chartsStore from './stores/charts.js';
import uiStore from './stores/ui.js';

import sidebar from './components/sidebar.js';
import navbar from './components/navbar.js';
import appSection from './components/appSection.js';
import mainContent from './components/mainContent.js';
import drawer from './components/drawer.js';
import appEditModal from './components/appEditModal.js';

window.Alpine = Alpine;

Alpine.store('dashboard', dashboardStore);
Alpine.store('charts', chartsStore);
Alpine.store('ui', uiStore);

Alpine.data('sidebar', sidebar);
Alpine.data('navbar', navbar);
Alpine.data('appSection', appSection);
Alpine.data('mainContent', mainContent);
Alpine.data('drawer', drawer);
Alpine.data('appEditModal', appEditModal);

Alpine.start();
Alpine.store('ui').init();
Alpine.store('dashboard').init();

window.addEventListener('beforeunload', () => {
    Alpine.store('charts')?.destroy();
});
