// Talkgroup-related functionality
import { formatTime, formatDuration, formatUnits } from '../utils.js';

import { getApiBaseUrl } from '../utils.js';
const API_BASE_URL = getApiBaseUrl();

// Fetch and display talkgroup activity
export async function fetchTalkgroupActivity() {
    try {
        const talkgroupId = document.getElementById('talkgroup-select').value;
        if (!talkgroupId) {
            document.querySelector('#talkgroup-activity .content').innerHTML = 
                '<div class="status-item">Select a talkgroup to view activity</div>';
            return;
        }

        const response = await fetch(`${API_BASE_URL}/calls/talkgroup/${talkgroupId}?limit=10`);
        if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);
        const data = await response.json();
        const content = document.querySelector('#talkgroup-activity .content');

        if (!data.data?.events || data.data.events.length === 0) {
            content.innerHTML = '<div class="status-item">No activity for this talkgroup</div>';
            return;
        }

        content.innerHTML = data.data.events.map(event => `
            <div class="status-item">
                <div>
                    <div>
                        ${event.activity_type === 'call' ? `
                            <span class="badge badge-info">Call</span>
                            ${event.emergency ? '<span class="badge badge-danger">Emergency</span>' : ''}
                            <div>Units: ${formatUnits(event.units)}</div>
                            <div>Duration: ${formatDuration(event.duration)}</div>
                        ` : event.activity_type === 'join' ? `
                            <span class="badge badge-warning">Join</span>
                            <div>Unit: ${event.unit} ${event.unit_alpha_tag ? `(${event.unit_alpha_tag})` : ''}</div>
                        ` : `
                            <span class="badge badge-secondary">${event.activity_type}</span>
                            <div>Unit: ${event.unit} ${event.unit_alpha_tag ? `(${event.unit_alpha_tag})` : ''}</div>
                        `}
                    </div>
                    <div class="timestamp">${formatTime(event.timestamp)}</div>
                </div>
            </div>
        `).join('');
    } catch (error) {
        document.querySelector('#talkgroup-activity .content').innerHTML = 
            `<div class="error">Error loading talkgroup activity: ${error.message}</div>`;
    }
}

// Get list of available talkgroups
export async function getTalkgroups() {
    try {
        const response = await fetch(`${API_BASE_URL}/talkgroups`);
        if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);
        const data = await response.json();
        return data.data || [];
    } catch (error) {
        console.error('Error fetching talkgroups:', error);
        return [];
    }
}

// Initialize talkgroup filters
export async function initializeTalkgroupFilters() {
    try {
        const talkgroups = await getTalkgroups();
        console.log('Talkgroups response:', talkgroups); // Debug log
        
        // If no talkgroups, show a message in the select
        if (talkgroups.length === 0) {
            const talkgroupOptions = '<option value="" disabled>No talkgroups available</option>';
            ['active-units-talkgroup-filter', 'talkgroup-select']
                .forEach(id => {
                    const select = document.getElementById(id);
                    if (select) {
                        select.innerHTML = talkgroupOptions;
                    }
                });
            return;
        }

        const talkgroupOptions = talkgroups.map(tg => 
            `<option value="${tg.talkgroup}">${tg.talkgroup} - ${tg.talkgroup_tag || 'Unknown'}</option>`
        ).join('');
        
        // Populate all talkgroup select elements
        const selects = {
            'active-units-talkgroup-filter': { includeAll: true },
            'talkgroup-select': { includeAll: true },
            'audio-talkgroup-select': { includeAll: false }
        };

        Object.entries(selects).forEach(([id, options]) => {
            const select = document.getElementById(id);
            if (select) {
                select.innerHTML = (options.includeAll ? '<option value="">All</option>' : '') + 
                    talkgroups.map(tg => {
                        const label = `${tg.talkgroup} - ${tg.talkgroup_tag || 'Unknown'}`;
                        const description = tg.talkgroup_description ? ` (${tg.talkgroup_description})` : '';
                        return `<option value="${tg.talkgroup}">${label}${description}</option>`;
                    }).join('');
            }
        });

        // Sort talkgroups in the audio select by talkgroup number
        const audioSelect = document.getElementById('audio-talkgroup-select');
        if (audioSelect) {
            const options = Array.from(audioSelect.options);
            options.sort((a, b) => {
                const aNum = parseInt(a.value);
                const bNum = parseInt(b.value);
                return aNum - bNum;
            });
            audioSelect.innerHTML = '';
            options.forEach(option => audioSelect.appendChild(option));
        }
    } catch (error) {
        console.error('Error initializing talkgroup filters:', error);
    }
}

// Export functions
window.fetchTalkgroupActivity = fetchTalkgroupActivity;
