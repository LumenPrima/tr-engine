import wsManager from '../websocket.js';
import { initializeApiConfig } from '../../utils.js';
import statusIndicators from './status-indicators.js';
import consoleManager from './console.js';
import statsManager from './stats.js';

// Initialize dashboard
export async function initDashboard() {
    await initializeApiConfig();
    wsManager.connect();

    // Subscribe to events with correct names
    wsManager.subscribe([
        'system.update',
        'system.rates',
        'system.config',
        'unit.activity',
        'unit.status',
        'call.start',
        'call.update',
        'call.end'
    ]);

    // Subscribe to transcriptions for all talkgroups
    wsManager.send({
        type: 'transcription.subscribe',
        data: {
            talkgroups: [] // Empty array means all talkgroups
        }
    });

    // Handle initial state
    wsManager.on('initial.state', (data) => {
        // Only update counts, not details
        if (data.systems) {
            statsManager.stats.systems = new Set(data.systems.map(s => s.id));
            statsManager.updateCount('systems', statsManager.stats.systems.size);
        }
        if (data.units) {
            statsManager.stats.units = new Set(data.units.filter(u => u.status.online).map(u => u.unit));
            statsManager.updateCount('units', statsManager.stats.units.size);
        }
        if (data.calls) {
            statsManager.stats.calls = new Set(data.calls.map(c => c.id));
            statsManager.updateCount('calls', statsManager.stats.calls.size);
        }
        consoleManager.setInitialState(data);
        statusIndicators.flash('initial.state');
    });

    // Handle system events
    wsManager.on('system.update', (data) => {
        statsManager.stats.systems = new Set(data.map(s => s.id));
        statsManager.updateCount('systems', statsManager.stats.systems.size);
        statsManager.updateActivity('systems');
        statsManager.flashCard('systems');
        statsManager.updateSystemDetails(data);
        statusIndicators.flash('system.update');
        consoleManager.addMessage('system.update', data);
    });

    wsManager.on('system.rates', (data) => {
        statusIndicators.flash('system.rates');
        consoleManager.addMessage('system.rates', data);
    });

    wsManager.on('system.config', (data) => {
        statusIndicators.flash('system.config');
        consoleManager.addMessage('system.config', data);
    });

    // Handle unit events
    wsManager.on('unit.activity', (data) => {
        const id = data.unit || data.id;
        if (data.active) {
            statsManager.stats.units.add(id);
        } else {
            statsManager.stats.units.delete(id);
        }
        statsManager.updateCount('units', statsManager.stats.units.size);
        statsManager.updateActivity('units');
        statsManager.flashCard('units');
        statusIndicators.flash('unit.activity');
        consoleManager.addMessage('unit.activity', data);
    });

    wsManager.on('unit.status', (data) => {
        if (data.units) {
            statsManager.updateUnitDetails(data.units);
        }
        statusIndicators.flash('unit.status');
        consoleManager.addMessage('unit.status', data);
    });

    // Handle call events
    wsManager.on('call.start', (data) => {
        statsManager.stats.calls.add(data.call_id || data.id);
        statsManager.updateCount('calls', statsManager.stats.calls.size);
        statsManager.updateActivity('calls');
        statsManager.flashCard('calls');
        if (data.calls) {
            statsManager.updateCallDetails(data.calls);
            statsManager.updateTalkgroupDetails(data.calls);
        }
        statusIndicators.flash('call.start');
        consoleManager.addMessage('call.start', data);
    });

    wsManager.on('call.update', (data) => {
        if (data.calls) {
            statsManager.updateCallDetails(data.calls);
            statsManager.updateTalkgroupDetails(data.calls);
        }
        statusIndicators.flash('call.update');
        consoleManager.addMessage('call.update', data);
    });

    wsManager.on('call.end', (data) => {
        statsManager.stats.calls.delete(data.call_id || data.id);
        statsManager.updateCount('calls', statsManager.stats.calls.size);
        statsManager.updateActivity('calls');
        statusIndicators.flash('call.end');
        consoleManager.addMessage('call.end', data);
    });

    // Handle transcription events
    wsManager.on('transcription.new', (data) => {
        statusIndicators.flash('transcription.new');
        consoleManager.addMessage('transcription.new', data);
    });
}

export default { initDashboard };
