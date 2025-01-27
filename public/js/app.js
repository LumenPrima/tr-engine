// Import modules
import { fetchSystemPerformance, initializeSystemFilters } from './modules/system.js';
import { fetchTranscriptionStats, fetchTalkgroupTranscriptions } from './modules/transcription.js';
import { fetchActiveCalls, fetchRecentHistory, fetchRecentCalls } from './modules/calls.js';
import { fetchActiveUnits } from './modules/units.js';
import { fetchTalkgroupActivity, initializeTalkgroupFilters } from './modules/talkgroups.js';

// Initialize the application
async function initializeApp() {
    try {
        // Initialize filters
        const systems = await initializeSystemFilters();
        await initializeTalkgroupFilters();

        // Initial data fetch
        await Promise.all([
            Promise.all([
                fetchSystemPerformance(),
                fetchTranscriptionStats()
            ]),
            fetchActiveCalls(),
            fetchActiveUnits(),
            fetchRecentHistory(),
            fetchTalkgroupActivity(),
            fetchRecentCalls()
        ]);

        // Set up refresh intervals
        setInterval(fetchSystemPerformance, 5000);  // Every 5 seconds
        setInterval(fetchActiveCalls, 3000);        // Every 3 seconds
        setInterval(fetchActiveUnits, 10000);       // Every 10 seconds
        setInterval(fetchRecentHistory, 30000);     // Every 30 seconds
        setInterval(fetchTalkgroupActivity, 15000); // Every 15 seconds
        setInterval(fetchRecentCalls, 30000);       // Every 30 seconds

        console.log('Application initialized successfully');
    } catch (error) {
        console.error('Error initializing application:', error);
    }
}

// Set up event listeners
function setupEventListeners() {
    // System filter changes
    document.querySelectorAll('select[id$="-system-filter"]').forEach(select => {
        select.addEventListener('change', () => {
            const containerId = select.id.replace('-system-filter', '');
            switch (containerId) {
                case 'active-calls':
                    fetchActiveCalls();
                    break;
                case 'active-units':
                    fetchActiveUnits();
                    break;
                case 'history':
                    fetchRecentHistory();
                    break;
                case 'recent-calls':
                    fetchRecentCalls();
                    break;
            }
        });
    });

    // Emergency filter changes
    document.querySelectorAll('input[id$="-emergency-filter"]').forEach(checkbox => {
        checkbox.addEventListener('change', () => {
            const containerId = checkbox.id.replace('-emergency-filter', '');
            switch (containerId) {
                case 'active-calls':
                    fetchActiveCalls();
                    break;
                case 'history':
                    fetchRecentHistory();
                    break;
            }
        });
    });

    // Talkgroup filter changes
    document.querySelectorAll('select[id$="-talkgroup-filter"]').forEach(select => {
        select.addEventListener('change', () => {
            const containerId = select.id.replace('-talkgroup-filter', '');
            switch (containerId) {
                case 'active-units':
                    fetchActiveUnits();
                    break;
            }
        });
    });

    // Talkgroup selection change
    const talkgroupSelect = document.getElementById('talkgroup-select');
    if (talkgroupSelect) {
        talkgroupSelect.addEventListener('change', async () => {
            const talkgroupId = talkgroupSelect.value;
            if (talkgroupId) {
                await fetchTalkgroupActivity();
                await fetchTalkgroupTranscriptions(talkgroupId);
            }
        });
    }

    // Transcription filter changes
    const transcriptionFilter = document.getElementById('recent-calls-transcription-filter');
    if (transcriptionFilter) {
        transcriptionFilter.addEventListener('change', fetchRecentCalls);
    }
}

// Refresh all data
export async function refreshAll() {
    try {
        await Promise.all([
            fetchSystemPerformance(),
            fetchActiveCalls(),
            fetchActiveUnits(),
            fetchRecentHistory(),
            fetchTalkgroupActivity(),
            fetchRecentCalls()
        ]);
        console.log('All data refreshed successfully');
    } catch (error) {
        console.error('Error refreshing data:', error);
    }
}

// Initialize when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    setupEventListeners();
    initializeApp();
});

// Export for use in HTML
window.refreshAll = refreshAll;
