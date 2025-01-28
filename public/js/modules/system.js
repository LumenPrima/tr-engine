// System-related functionality
import { getApiBaseUrl } from '../utils.js';
const API_BASE_URL = getApiBaseUrl();

// Fetch and display system performance
export async function fetchSystemPerformance() {
    try {
        const response = await fetch(`${API_BASE_URL}/systems/performance`);
        if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);
        const data = await response.json();
        const content = document.querySelector('#system-performance .content');
        
        if (!data.stats) {
            content.innerHTML = '<div class="status-item">No performance data available</div>';
            return;
        }

        const stats = data.stats;
        content.innerHTML = `
            <div class="stat-group">
                <div class="stat-box">
                    <div class="stat-value">${stats.active_systems}/${stats.total_systems}</div>
                    <div class="stat-label">Active Systems</div>
                </div>
                <div class="stat-box">
                    <div class="stat-value">${stats.aggregate.decoderate.toFixed(1)}/s</div>
                    <div class="stat-label">Avg Decode Rate</div>
                </div>
                <div class="stat-box">
                    <div class="stat-value">${stats.aggregate.active_recorders}/${stats.aggregate.total_recorders}</div>
                    <div class="stat-label">Active Recorders</div>
                </div>
            </div>
            <div style="margin-top: 15px;">
                ${stats.systems.slice(0, 10).map(system => `
                    <div class="status-item">
                        <div>
                            <div>
                                <span class="performance-indicator ${
                                    system.current_decoderate > 80 ? 'performance-good' :
                                    system.current_decoderate > 40 ? 'performance-medium' :
                                    'performance-poor'
                                }"></span>
                                ${system.sys_name}
                            </div>
                            <div>Decode Rate: ${system.current_decoderate?.toFixed(1) || '0'}/s</div>
                            <div>Recorders: ${system.active_recorders?.length || 0}/${system.config?.control_channels?.length || 0}</div>
                        </div>
                    </div>
                `).join('')}
            </div>
        `;
    } catch (error) {
        document.querySelector('#system-performance .content').innerHTML = 
            `<div class="error">Error loading system performance: ${error.message}</div>`;
    }
}

// Get list of available systems
export async function getSystems() {
    try {
        const response = await fetch(`${API_BASE_URL}/systems`);
        if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);
        const data = await response.json();
        return data.systems || [];
    } catch (error) {
        console.error('Error fetching systems:', error);
        return [];
    }
}

// Initialize system filters
export async function initializeSystemFilters() {
    try {
        const systems = await getSystems();
        console.log('Systems response:', systems); // Debug log
        
        const systemOptions = systems.map(sys => 
            `<option value="${sys.sys_name}">${sys.sys_name}</option>`
        ).join('');
        
        ['active-calls-system-filter', 'active-units-system-filter', 'history-system-filter', 'recent-calls-system-filter']
            .forEach(id => {
                const select = document.getElementById(id);
                if (select) {
                    select.innerHTML = '<option value="">All</option>' + systemOptions;
                }
            });

        return systems;
    } catch (error) {
        console.error('Error initializing system filters:', error);
        return [];
    }
}
