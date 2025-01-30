import wsManager from './websocket.js';
import { initializeApiConfig, getApiBaseUrl } from '../utils.js';

class StatusIndicatorManager {
    constructor() {
        this.statusBar = document.getElementById('status-bar');
        this.messageTypes = new Map();
    }

    flash(type) {
        let indicator = this.messageTypes.get(type);
        if (!indicator) {
            indicator = document.createElement('div');
            indicator.className = 'status-indicator';
            indicator.textContent = type;
            this.statusBar.appendChild(indicator);
            this.messageTypes.set(type, indicator);
        }
        indicator.classList.remove('flash');
        void indicator.offsetWidth; // Force reflow
        indicator.classList.add('flash');
    }
}

class StatsManager {
    constructor() {
        this.eventCount = 0;
        this.lastEventTime = Date.now();
        this.eventsPerSecond = 0;
        
        // Update events per second every second
        setInterval(() => {
            const now = Date.now();
            const elapsed = (now - this.lastEventTime) / 1000;
            this.eventsPerSecond = Math.round(this.eventCount / elapsed);
            this.eventCount = 0;
            this.lastEventTime = now;
            this.updateStats();
        }, 1000);
    }

    recordEvent() {
        this.eventCount++;
    }

    updateStats() {
        document.getElementById('events-rate').textContent = `${this.eventsPerSecond}/s`;
    }
}

class UnitTracker {
    constructor() {
        this.units = new Map();
        this.talkgroups = new Map();
        this.container = document.getElementById('unit-groups-container');
        this.unassociatedContainer = document.querySelector('#unassociated-group .units-container');
        this.statusManager = new StatusIndicatorManager();
        this.statsManager = new StatsManager();
        this.init();
    }

    updateStats() {
        // Update unit counts
        const activeUnits = this.units.size;
        const activeTalkgroups = this.talkgroups.size;
        
        // Count units by status
        let transmittingCount = 0;
        let emergencyCount = 0;
        this.units.forEach(unit => {
            if (unit.element.classList.contains('transmitting')) transmittingCount++;
            if (unit.element.classList.contains('emergency')) emergencyCount++;
        });

        // Update display
        document.getElementById('units-count').textContent = activeUnits;
        document.getElementById('talkgroups-count').textContent = activeTalkgroups;
        document.getElementById('transmitting-count').textContent = transmittingCount;
        document.getElementById('emergency-count').textContent = emergencyCount;
    }

    async init() {
        // Initialize API configuration
        await initializeApiConfig();
        
        // Subscribe to unit events
        wsManager.subscribe(['unit.activity', 'unit.status']);
        wsManager.on('unit.activity', data => this.handleUnitActivity(data));
        wsManager.on('unit.status', data => this.updateUnitStatus(data.unit_id, data.status));

        // Connect WebSocket
        wsManager.connect();

        // Load talkgroups first, then units
        await this.loadTalkgroups();
        await this.loadUnits();
    }

    async loadTalkgroups() {
        try {
            const response = await fetch(`${getApiBaseUrl()}/talkgroups?limit=100`);
            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }
            const data = await response.json();
            if (data.data) {
                // Initialize talkgroup data map
                this.talkgroupData = new Map();
                data.data.forEach(tg => {
                    this.talkgroupData.set(tg.talkgroup, {
                        id: tg.talkgroup,
                        tag: tg.talkgroup_tag,
                        description: tg.description,
                        category: tg.category
                    });
                });
                this.statusManager.flash('talkgroups.loaded');
            }
        } catch (error) {
            console.error('Error loading talkgroups:', error);
            // Create placeholder for unassociated units if API fails
            if (!this.talkgroups.size) {
                this.addTalkgroup({ id: 'unassociated', tag: 'Unassociated Units' });
            }
        }
    }

    async loadUnits() {
        try {
            const response = await fetch(`${getApiBaseUrl()}/units/active?window=30`);
            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }
            const data = await response.json();
            if (data.data?.units) {
                data.data.units.forEach(unit => this.addOrUpdateUnit({
                    id: unit.unit,
                    tag: unit.unit_alpha_tag,
                    status: unit.status.online ? 'active' : 'inactive',
                    talkgroup_id: unit.status.current_talkgroup,
                    talkgroup_tag: unit.status.current_talkgroup_tag,
                    last_seen: unit.status.last_seen
                }));
                this.statusManager.flash('units.loaded');
            }
        } catch (error) {
            console.error('Error loading units:', error);
        }
    }

    pulseGroupAura(talkgroupId, status) {
        const talkgroup = this.talkgroups.get(talkgroupId);
        if (!talkgroup) return;

        const element = talkgroup.element;
        const auraClass = `aura-${status}`;

        // Remove any existing aura classes
        element.classList.remove('aura-active', 'aura-transmitting', 'aura-emergency');
        
        // Force reflow
        void element.offsetWidth;
        
        // Add new aura class
        element.classList.add(auraClass);
        
        // Remove class after animation completes (matching the 3s CSS animation)
        setTimeout(() => {
            element.classList.remove(auraClass);
        }, 3000);
    }

    handleUnitActivity(data) {
        const unit = {
            id: data.unit,
            tag: data.unit_alpha_tag,
            status: data.activity_type === 'call' ? 'transmitting' : 'active',
            talkgroup_id: data.talkgroup,
            talkgroup_tag: data.talkgroup_tag,
            last_seen: data.status.last_seen
        };

        const unitElement = this.units.get(unit.id)?.element;
        if (unitElement) {
            // Add pale class on activity end
            if (data.activity_type === 'end') {
                unitElement.classList.add('pale');
            } else {
                // Remove pale class on any other activity
                unitElement.classList.remove('pale');
                // Pulse talkgroup aura for non-end activities
                if (unit.talkgroup_id) {
                    this.pulseGroupAura(unit.talkgroup_id, unit.status);
                }
            }
        }

        this.addOrUpdateUnit(unit);
        this.statusManager.flash(`unit.activity.${data.activity_type}`);
        this.statsManager.recordEvent();
        this.updateStats();
    }

    ensureTalkgroupExists(talkgroupId) {
        if (this.talkgroups.has(talkgroupId)) return this.talkgroups.get(talkgroupId);

        const talkgroup = this.talkgroupData?.get(talkgroupId) || {
            id: talkgroupId,
            tag: `Talkgroup ${talkgroupId}`,
            description: '',
            category: ''
        };

        const groupElement = document.createElement('div');
        groupElement.className = 'unit-group';
        groupElement.dataset.talkgroupId = talkgroupId;
        groupElement.innerHTML = `
            <h2>
                ${talkgroup.tag || `Talkgroup ${talkgroup.id}`}
                ${talkgroup.description ? `<div class="talkgroup-description">${talkgroup.description}</div>` : ''}
                ${talkgroup.category ? `<div class="talkgroup-category">${talkgroup.category}</div>` : ''}
            </h2>
            <div class="units-container"></div>
        `;

        this.container.appendChild(groupElement);
        const talkgroupInfo = {
            element: groupElement,
            unitsContainer: groupElement.querySelector('.units-container'),
            data: talkgroup,
            unitCount: 0
        };
        this.talkgroups.set(talkgroupId, talkgroupInfo);
        this.statusManager.flash('talkgroup.created');
        return talkgroupInfo;
    }

    removeTalkgroupIfEmpty(talkgroupId) {
        const talkgroup = this.talkgroups.get(talkgroupId);
        if (talkgroup && talkgroup.unitCount <= 0) {
            talkgroup.element.remove();
            this.talkgroups.delete(talkgroupId);
            this.statusManager.flash('talkgroup.removed');
        }
    }

    addOrUpdateUnit(unit) {
        let unitElement = this.units.get(unit.id)?.element;
        
        if (!unitElement) {
            unitElement = document.createElement('div');
            unitElement.className = 'unit-dot';
            unitElement.setAttribute('data-unit-id', unit.id);
            
            // Add click handler for additional info
            unitElement.addEventListener('click', (e) => this.showUnitDetails(unit, e));
            
            this.units.set(unit.id, { element: unitElement, data: unit });
            this.statusManager.flash('unit.added');
        } else {
            // Update stored data
            this.units.get(unit.id).data = { ...this.units.get(unit.id).data, ...unit };
            this.statusManager.flash('unit.updated');
        }

        // Update status class
        this.updateUnitStatus(unit.id, unit.status);

        // Handle container changes
        const currentContainer = unitElement.parentElement;
        let targetContainer;
        let oldTalkgroupId = null;

        // Find the old talkgroup ID if the unit is moving
        if (currentContainer && currentContainer !== this.unassociatedContainer) {
            const oldTalkgroupElement = currentContainer.closest('.unit-group');
            if (oldTalkgroupElement) {
                oldTalkgroupId = oldTalkgroupElement.dataset.talkgroupId;
            }
        }

        // Get or create target container
        if (unit.talkgroup_id) {
            const talkgroup = this.ensureTalkgroupExists(unit.talkgroup_id);
            targetContainer = talkgroup.unitsContainer;
        } else {
            targetContainer = this.unassociatedContainer;
        }

        // Move unit to new container if needed
        if (currentContainer !== targetContainer) {
            // Decrement count from old talkgroup
            if (oldTalkgroupId) {
                const oldTalkgroup = this.talkgroups.get(oldTalkgroupId);
                if (oldTalkgroup) {
                    oldTalkgroup.unitCount--;
                    this.removeTalkgroupIfEmpty(oldTalkgroupId);
                }
            }

            // Increment count in new talkgroup
            if (unit.talkgroup_id) {
                const newTalkgroup = this.talkgroups.get(unit.talkgroup_id);
                if (newTalkgroup) {
                    newTalkgroup.unitCount++;
                }
            }

            targetContainer.appendChild(unitElement);
            this.statusManager.flash('unit.moved');
        }
    }

    updateUnitStatus(unitId, status) {
        const unit = this.units.get(unitId);
        if (!unit) return;

        const element = unit.element;
        const oldStatus = element.className.replace('unit-dot', '').trim();
        
        // Remove all status classes
        element.classList.remove('active', 'inactive', 'transmitting', 'emergency');
        
        // Remove pale class and add appropriate status class
        element.classList.remove('pale');
        switch (status) {
            case 'active':
                element.classList.add('active');
                break;
            case 'transmitting':
                element.classList.add('transmitting');
                break;
            case 'emergency':
                element.classList.add('emergency');
                break;
            default:
                element.classList.add('inactive');
        }

        // Only flash if status actually changed
        if (oldStatus !== status) {
            this.statusManager.flash(`unit.status.${status}`);
            this.statsManager.recordEvent();
            this.updateStats();
        }
    }

    showUnitDetails(unit, event) {
        const details = {
            'Unit ID': unit.id,
            'Unit Tag': unit.tag || 'None',
            'Status': unit.status,
            'Talkgroup': unit.talkgroup_tag || unit.talkgroup_id || 'Unassociated',
            'Last Seen': new Date(unit.last_seen).toLocaleTimeString()
        };
        
        // Show tooltip with details
        const tooltip = document.createElement('div');
        tooltip.className = 'unit-details-tooltip';
        tooltip.style.cssText = `
            position: fixed;
            background: rgba(0, 0, 0, 0.9);
            color: white;
            padding: 10px;
            border-radius: 4px;
            z-index: 1000;
            font-size: 14px;
        `;
        
        tooltip.innerHTML = Object.entries(details)
            .map(([key, value]) => `<div><strong>${key}:</strong> ${value}</div>`)
            .join('');
        
        document.body.appendChild(tooltip);
        
        // Position tooltip near cursor
        const positionTooltip = (e) => {
            const padding = 10;
            tooltip.style.left = `${e.clientX + padding}px`;
            tooltip.style.top = `${e.clientY + padding}px`;
        };
        
        // Initial position
        positionTooltip(event);
        
        // Remove tooltip after a delay
        setTimeout(() => {
            document.body.removeChild(tooltip);
        }, 3000);

        this.statusManager.flash('unit.details');
    }
}

// Initialize the tracker when the page loads
document.addEventListener('DOMContentLoaded', () => {
    window.unitTracker = new UnitTracker();
});
