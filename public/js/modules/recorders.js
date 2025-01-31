import wsManager from './websocket.js';
import { initializeApiConfig } from '../utils.js';

// State management
const state = {
    recorders: new Map(),
    stats: {
        total: 0,
        recording: 0,
        idle: 0,
        available: 0
    }
};

// UI Elements
let elements;

function initElements() {
    elements = {
        activity: document.getElementById('recorders-activity'),
        totalCount: document.getElementById('total-count'),
        recordingCount: document.getElementById('recording-count'),
        idleCount: document.getElementById('idle-count'),
        availableCount: document.getElementById('available-count'),
        recordersList: document.getElementById('recorders-list'),
        eventsConsole: document.getElementById('events-console')
    };
}

// Format frequency in MHz
function formatFrequency(freq) {
    return (freq / 1000000).toFixed(4);
}

// Format duration in MM:SS
function formatDuration(duration) {
    const minutes = Math.floor(duration / 60);
    const seconds = Math.floor(duration % 60);
    return `${minutes.toString().padStart(2, '0')}:${seconds.toString().padStart(2, '0')}`;
}

// Cache for talkgroup details
const talkgroupCache = new Map();

// Fetch talkgroup details from API
async function fetchTalkgroupDetails(talkgroup) {
    if (talkgroupCache.has(talkgroup)) {
        return talkgroupCache.get(talkgroup);
    }

    try {
        const response = await fetch(`/api/v1/talkgroups/${talkgroup}`);
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        const data = await response.json();
        if (data.status === 'success') {
            talkgroupCache.set(talkgroup, data.data);
            return data.data;
        }
    } catch (error) {
        console.error('Error fetching talkgroup details:', error);
    }
    return null;
}

// Format talkgroup info
async function formatTalkgroup(recorder) {
    try {
        if (recorder?.current_call?.talkgroup) {
            const call = recorder.current_call;
            
            // Fetch additional talkgroup details
            const details = await fetchTalkgroupDetails(call.talkgroup);
            
            let tag = call.talkgroup.toString();
            let description = '';
            let category = '';
            
            if (details) {
                tag = `${details.talkgroup} (${details.talkgroup_tag})`;
                description = details.description;
                category = details.category;
            } else if (call.talkgroup_alpha_tag) {
                tag = `${call.talkgroup} (${call.talkgroup_alpha_tag})`;
                description = call.talkgroup_description || '';
            }
        
            const classes = [
                call.emergency ? 'emergency' : '',
                recorder.rec_state_type !== 'RECORDING' ? 'inactive' : ''
            ].filter(Boolean).join(' ');
            
            return `<div class="talkgroup ${classes}">
                <div class="talkgroup-tag">${tag}</div>
                ${description ? `<div class="talkgroup-desc">${description}</div>` : ''}
                ${category ? `<div class="talkgroup-category">${category}</div>` : ''}
            </div>`;
        }
        return '-';
    } catch (error) {
        console.error('Error formatting talkgroup:', error);
        return '-';
    }
}

// Update stats display
function updateStats(stats) {
    state.stats = stats;
    elements.totalCount.textContent = stats.total;
    elements.recordingCount.textContent = stats.recording;
    elements.idleCount.textContent = stats.idle;
    elements.availableCount.textContent = stats.available;
}

// Create or update a recorder row
async function updateRecorderRow(recorder) {
    let row = document.getElementById(`recorder-${recorder.id}`);
    if (!row) {
        row = document.createElement('tr');
        row.id = `recorder-${recorder.id}`;
        
        // Find the correct position to insert the new row
        if (!recorder?.id) {
            elements.recordersList.appendChild(row);
            return;
        }

        const [newSystem, newNum] = recorder.id.split('_').map(Number);
        const rows = Array.from(elements.recordersList.children);
        const insertIndex = rows.findIndex(existingRow => {
            const existingId = existingRow.id.replace('recorder-', '');
            if (!existingId) return false;
            
            const [existingSystem, existingNum] = existingId.split('_').map(Number);
            return (existingSystem > newSystem) || 
                   (existingSystem === newSystem && existingNum > newNum);
        });
        
        if (insertIndex === -1) {
            elements.recordersList.appendChild(row);
        } else {
            elements.recordersList.insertBefore(row, rows[insertIndex]);
        }
    }

    row.className = `recorder-row ${recorder.rec_state_type.toLowerCase()}`;
    
    // Update all fields except talkgroup first
    // Format ID to show just the number (e.g., "01" instead of "0_1")
    const [system, num] = recorder.id.split('_').map(Number);
    const formattedId = num.toString().padStart(2, '0');
    
    row.innerHTML = `
        <td>${formattedId}</td>
        <td>
            <span class="status-indicator status-${recorder.rec_state_type.toLowerCase()}">
                ${recorder.rec_state_type}
            </span>
        </td>
        <td>${formatDuration(recorder.duration)}</td>
        <td>${recorder.count}</td>
        <td id="talkgroup-${recorder.id}">Loading...</td>
    `;

    // Update talkgroup info asynchronously
    const talkgroupCell = row.querySelector(`#talkgroup-${recorder.id}`);
    talkgroupCell.innerHTML = await formatTalkgroup(recorder);

    // Store state
    state.recorders.set(recorder.id, recorder);
}

// Initialize recorders table
async function initRecordersList(recorders) {
    elements.recordersList.innerHTML = '';
    state.recorders.clear();
    
    const sortedRecorders = [...recorders].sort((a, b) => {
        // Handle cases where recorder or id might be undefined
        if (!a?.id || !b?.id) return 0;
        
        // Split IDs into parts (e.g. "0_2" -> [0, 2])
        const [aSystem, aNum] = a.id.split('_').map(Number);
        const [bSystem, bNum] = b.id.split('_').map(Number);
        
        // Sort by system ID first, then by recorder number
        if (aSystem !== bSystem) {
            return aSystem - bSystem;
        }
        return aNum - bNum;
    });
    for (const recorder of sortedRecorders) {
        await updateRecorderRow(recorder);
    }
}

// Add event to console
function addEventMessage(data) {
    const message = document.createElement('div');
    message.className = 'event-message new';
    
    const timestamp = new Date().toLocaleTimeString();
    message.innerHTML = `
        <span class="timestamp">${timestamp}</span>
        <span class="recorder-id">${data.id}</span>
        <span class="state-change">${data.previousState} → ${data.newState}</span>
        <span class="freq">${formatFrequency(data.freq)} MHz</span>
    `;
    
    elements.eventsConsole.insertBefore(message, elements.eventsConsole.firstChild);
    
    // Remove new class after animation
    setTimeout(() => message.classList.remove('new'), 1000);
    
    // Limit console entries
    while (elements.eventsConsole.children.length > 100) {
        elements.eventsConsole.removeChild(elements.eventsConsole.lastChild);
    }
}

// Flash activity indicator
function flashActivity() {
    elements.activity.classList.remove('active');
    void elements.activity.offsetWidth; // Force reflow
    elements.activity.classList.add('active');
    
    setTimeout(() => {
        elements.activity.classList.remove('active');
    }, 1000);
}

// Initialize recorders page
export async function initRecorders() {
    await initializeApiConfig();
    initElements();
    wsManager.connect();

    // Subscribe to recorder events
    wsManager.subscribe(['recorder.stateChange', 'recorders.status', 'recorder.update']);

    // Handle initial state
    wsManager.on('initial.state', (data) => {
        if (data.recorders) {
            updateStats(data.recorders.stats);
            initRecordersList(data.recorders.states);
        }
    });

    // Handle recorder state changes
    wsManager.on('recorder.stateChange', (data) => {
        addEventMessage(data);
        flashActivity();
    });

    // Handle individual recorder updates
    wsManager.on('recorder.update', (data) => {
        if (data) {
            updateRecorderRow(data);
            flashActivity();
        }
    });

    // Handle recorder stats updates
    wsManager.on('recorders.status', (data) => {
        updateStats(data);
        flashActivity();
    });

    // Request initial recorder status
    wsManager.send({
        type: 'get.recorder_stats'
    });
}

export default { initRecorders };
