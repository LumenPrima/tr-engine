// Unit-related functionality
import { formatTime, formatUnits } from '../utils.js';

import { getApiBaseUrl } from '../utils.js';
const API_BASE_URL = getApiBaseUrl();

// Fetch and display active units
export async function fetchActiveUnits() {
    try {
        const systemFilter = document.getElementById('active-units-system-filter').value;
        const talkgroupFilter = document.getElementById('active-units-talkgroup-filter').value;
        
        let url = `${API_BASE_URL}/units/active`;
        const params = new URLSearchParams();
        if (systemFilter) params.append('sys_name', systemFilter);
        if (talkgroupFilter) params.append('talkgroup', talkgroupFilter);
        if (params.toString()) url += '?' + params.toString();

        const response = await fetch(url);
        if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);
        const data = await response.json();
        const content = document.querySelector('#active-units .content');
        
        if (!data.data?.units || data.data.units.length === 0) {
            content.innerHTML = '<div class="status-item">No active units</div>';
            return;
        }

        content.innerHTML = data.data.units.slice(0, 10).map(unit => `
            <div class="status-item">
                <div>
                    <div>
                        Unit: ${unit.unit} ${unit.unit_alpha_tag ? `(${unit.unit_alpha_tag})` : ''}
                        ${unit.status.online ? 
                            '<span class="badge badge-success">Online</span>' : 
                            '<span class="badge badge-warning">Offline</span>'
                        }
                    </div>
                    <div>Current TG: ${unit.status.current_talkgroup || 'None'} ${unit.status.current_talkgroup_tag ? `(${unit.status.current_talkgroup_tag})` : ''}</div>
                    <div class="timestamp">Last Seen: ${formatTime(unit.status.last_seen)}</div>
                    <div id="unit-${unit.unit}-details"></div>
                </div>
                <button class="button" onclick="viewUnitHistory(${unit.unit})">View History</button>
            </div>
        `).join('');

        // Update count in header
        const header = document.querySelector('#active-units h2');
        header.innerHTML = `Active Units <span class="count">${data.data.units.length}</span>`;
    } catch (error) {
        document.querySelector('#active-units .content').innerHTML = 
            `<div class="error">Error loading active units: ${error.message}</div>`;
    }
}

// View unit history
export async function viewUnitHistory(unitId) {
    try {
        const response = await fetch(`${API_BASE_URL}/units/${unitId}/history?limit=10`);
        if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);
        const data = await response.json();
        
        const historyHtml = data.data.history.map(entry => `
            <div class="status-item">
                <div>
                    <div>${entry.activity_type.toUpperCase()}</div>
                    ${entry.activity_type === 'call' ? `
                        <div>Talkgroup: ${entry.talkgroup || 'Unknown'} ${entry.talkgroup_tag ? `(${entry.talkgroup_tag})` : ''}</div>
                        ${entry.emergency ? '<span class="badge badge-danger">Emergency</span>' : ''}
                    ` : ''}
                    ${entry.activity_type === 'on' || entry.activity_type === 'off' ? `
                        <div>Status: ${entry.activity_type === 'on' ? 'Online' : 'Offline'}</div>
                    ` : ''}
                    ${entry.activity_type === 'join' ? `
                        <div>Joined TG: ${entry.talkgroup} ${entry.talkgroup_tag ? `(${entry.talkgroup_tag})` : ''}</div>
                    ` : ''}
                    <div class="timestamp">${formatTime(entry.timestamp)}</div>
                </div>
            </div>
        `).join('');

        const unitDetails = document.querySelector(`#unit-${unitId}-details`);
        if (unitDetails) {
            unitDetails.innerHTML = `
                <div class="unit-details">
                    <h4>Recent History</h4>
                    ${historyHtml}
                </div>
            `;
        }
    } catch (error) {
        console.error('Error fetching unit history:', error);
        const unitDetails = document.querySelector(`#unit-${unitId}-details`);
        if (unitDetails) {
            unitDetails.innerHTML = `
                <div class="unit-details">
                    <div class="error">Error loading unit history: ${error.message}</div>
                </div>
            `;
        }
    }
}

// Export functions
window.fetchActiveUnits = fetchActiveUnits;
window.viewUnitHistory = viewUnitHistory;
