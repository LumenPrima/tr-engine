// Import modules
import { fetchSystemPerformance, initializeSystemFilters } from './modules/system.js';
import { fetchTranscriptionStats, fetchTalkgroupTranscriptions } from './modules/transcription.js';
import { fetchActiveCalls, fetchRecentHistory, fetchRecentCalls } from './modules/calls.js';
import { fetchActiveUnits } from './modules/units.js';
import { fetchTalkgroupActivity, initializeTalkgroupFilters } from './modules/talkgroups.js';
import wsManager from './modules/websocket.js';
import { initializeApiConfig } from './utils.js';

// Initialize the application
async function initializeApp() {
    try {
        console.log('Initializing application...');
        
        // Initialize API configuration first
        console.log('Initializing API configuration...');
        await initializeApiConfig();
        console.log('API configuration initialized');

        // Initialize WebSocket connection
        wsManager.connect();

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

        // Subscribe to WebSocket events
        wsManager.subscribe([
            'calls',
            'units',
            'systems',
            'transcription'
        ]);

        // Set up refresh intervals for non-WebSocket data
        setInterval(fetchTranscriptionStats, 30000);  // Every 30 seconds
        setInterval(fetchRecentHistory, 30000);       // Every 30 seconds
        setInterval(fetchRecentCalls, 30000);         // Every 30 seconds

        // Set up WebSocket event handlers
        wsManager.on('state.update', handleStateUpdate);
        wsManager.on('audio.new', handleNewAudio);

        console.log('Application initialized successfully');
    } catch (error) {
        console.error('Error initializing application:', error);
    }
}

// Handle state updates from WebSocket
function handleStateUpdate(data) {
    switch (data.resource) {
        case 'calls':
            handleCallsUpdate(data);
            break;
        case 'units':
            handleUnitsUpdate(data);
            break;
        case 'systems':
            handleSystemsUpdate(data);
            break;
    }
}

function handleCallsUpdate(data) {
    if (data.action === 'add' || data.action === 'update') {
        const container = document.querySelector('#active-calls .content');
        if (!container) return;

        // Create or update call element
        let callElement = document.getElementById(`call-${data.payload.call_id}`);
        if (!callElement) {
            callElement = document.createElement('div');
            callElement.id = `call-${data.payload.call_id}`;
            callElement.className = 'status-item';
            container.insertBefore(callElement, container.firstChild);
        }

        // Update content
        callElement.innerHTML = `
            <div>
                <div>
                    <span class="badge badge-info">Call</span>
                    ${data.payload.emergency ? '<span class="badge badge-danger">Emergency</span>' : ''}
                    <div>Talkgroup: ${data.payload.talkgroup_tag || data.payload.talkgroup}</div>
                    <div>Units: ${data.payload.units?.join(', ') || 'None'}</div>
                </div>
                <div class="timestamp">${new Date(data.payload.start_time * 1000).toLocaleTimeString()}</div>
            </div>
        `;
    } else if (data.action === 'remove') {
        const element = document.getElementById(`call-${data.payload.call_id}`);
        if (element) {
            element.remove();
        }
    }
}

function handleUnitsUpdate(data) {
    if (data.action === 'add' || data.action === 'update') {
        const container = document.querySelector('#active-units .content');
        if (!container) return;

        // Create or update unit element
        let unitElement = document.getElementById(`unit-${data.payload.unit_id}`);
        if (!unitElement) {
            unitElement = document.createElement('div');
            unitElement.id = `unit-${data.payload.unit_id}`;
            unitElement.className = 'status-item';
            container.insertBefore(unitElement, container.firstChild);
        }

        // Update content
        unitElement.innerHTML = `
            <div>
                <div>
                    <span class="badge badge-info">Unit</span>
                    <div>ID: ${data.payload.unit_id}</div>
                    <div>Status: ${data.payload.status}</div>
                    ${data.payload.talkgroup ? `<div>Talkgroup: ${data.payload.talkgroup_tag || data.payload.talkgroup}</div>` : ''}
                </div>
                <div class="timestamp">${new Date(data.payload.last_update * 1000).toLocaleTimeString()}</div>
            </div>
        `;
    } else if (data.action === 'remove') {
        const element = document.getElementById(`unit-${data.payload.unit_id}`);
        if (element) {
            element.remove();
        }
    }
}

function handleSystemsUpdate(data) {
    if (data.action === 'add' || data.action === 'update') {
        const container = document.querySelector('#system-performance .content');
        if (!container) return;

        // Create or update system element
        let systemElement = document.getElementById(`system-${data.payload.system_id}`);
        if (!systemElement) {
            systemElement = document.createElement('div');
            systemElement.id = `system-${data.payload.system_id}`;
            systemElement.className = 'status-item';
            container.insertBefore(systemElement, container.firstChild);
        }

        // Calculate performance indicator
        const performanceClass = data.payload.status === 'online' ? 
            'performance-good' : data.payload.status === 'degraded' ? 
            'performance-medium' : 'performance-poor';

        // Update content
        systemElement.innerHTML = `
            <div>
                <div>
                    <span class="performance-indicator ${performanceClass}"></span>
                    <span>${data.payload.name}</span>
                    <div>Status: ${data.payload.status}</div>
                    ${data.payload.message ? `<div>Message: ${data.payload.message}</div>` : ''}
                </div>
                <div class="timestamp">${new Date(data.payload.last_update * 1000).toLocaleTimeString()}</div>
            </div>
        `;
    } else if (data.action === 'remove') {
        const element = document.getElementById(`system-${data.payload.system_id}`);
        if (element) {
            element.remove();
        }
    }
}

// Handle new audio from WebSocket
function handleNewAudio({ audio, metadata }) {
    // Update UI to show new audio
    const audioContainer = document.getElementById('audio-container');
    if (!audioContainer) return;

    const audioElement = audio;
    audioElement.controls = true;
    audioElement.preload = 'auto';
    audioElement.crossOrigin = 'anonymous';
    
    const audioWrapper = document.createElement('div');
    audioWrapper.className = 'audio-item';
    
    const metadataDiv = document.createElement('div');
    metadataDiv.className = 'audio-metadata';
    metadataDiv.innerHTML = `
        <strong>Talkgroup:</strong> ${metadata.talkgroup_tag || metadata.talkgroup}<br>
        <strong>Time:</strong> ${new Date(metadata.start_time * 1000).toLocaleTimeString()}<br>
        ${metadata.emergency ? '<span class="emergency-badge">Emergency</span>' : ''}
    `;
    
    audioWrapper.appendChild(metadataDiv);
    audioWrapper.appendChild(audioElement);
    
    // Add to container
    audioContainer.insertBefore(audioWrapper, audioContainer.firstChild);
    
    // Keep only last 10 audio elements
    while (audioContainer.children.length > 10) {
        audioContainer.removeChild(audioContainer.lastChild);
    }

    // Auto-play if enabled
    if (document.getElementById('auto-play-audio')?.checked) {
        audioElement.play().catch(error => {
            console.error('Error playing audio:', error);
            // Try playing again after a short delay
            setTimeout(() => {
                audioElement.play().catch(console.error);
            }, 100);
        });
    }
}

// Set up event listeners
function setupEventListeners() {
    // Audio controls
    const audioControls = document.getElementById('audio-controls');
    if (audioControls) {
        const talkgroupSelect = audioControls.querySelector('#audio-talkgroup-select');
        const subscribeBtn = audioControls.querySelector('#subscribe-audio-btn');
        const unsubscribeBtn = audioControls.querySelector('#unsubscribe-audio-btn');
        
        if (subscribeBtn) {
            subscribeBtn.addEventListener('click', () => {
                const selectedTalkgroups = Array.from(talkgroupSelect.selectedOptions).map(opt => parseInt(opt.value));
                if (selectedTalkgroups.length > 0) {
                    wsManager.subscribeToAudio(selectedTalkgroups, {
                        format: 'm4a',
                        options: {
                            emergencyOnly: document.getElementById('emergency-only-audio')?.checked || false,
                            includeMetadata: true
                        }
                    });
                }
            });
        }
        
        if (unsubscribeBtn) {
            unsubscribeBtn.addEventListener('click', () => {
                const selectedTalkgroups = Array.from(talkgroupSelect.selectedOptions).map(opt => parseInt(opt.value));
                wsManager.unsubscribeFromAudio(selectedTalkgroups);
            });
        }
    }

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
