import wsManager from './websocket.js';
import { initializeApiConfig, getApiBaseUrl } from '../utils.js';

class StatusIndicatorManager {
    constructor() {
        this.statusBar = document.getElementById('status-bar');
        this.messageTypes = new Map();
    }

    flash(type) {
        // Only show unit-related events
        if (!type.startsWith('unit.') && !type.startsWith('talkgroup.')) {
            return;
        }

        let indicator = this.messageTypes.get(type);
        if (!indicator) {
            indicator = document.createElement('div');
            indicator.className = 'status-indicator';
            indicator.textContent = type;
            this.statusBar.appendChild(indicator);
            this.messageTypes.set(type, indicator);
        }

        // Reset animation
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
        this.statusManager = new StatusIndicatorManager();
        this.statsManager = new StatsManager();
        
        // Constants for timing states
        this.RECENT_TIMEOUT = 10000;  // 10 seconds for recent activity
        this.STALE_TIMEOUT = 60000;   // 1 minute for stale state
        
        // Sorting and filtering state
        this.sortBy = 'id';
        this.sortAsc = true;
        this.timeFilter = 0; // 0 means show all
        
        // Initialize controls
        this.initSortControls();
        this.initTimeFilter();
        
        // Run state updates every 5 seconds
        setInterval(() => {
            this.updateUnitStates();
            this.cleanupEmptyTalkgroups();
        }, 5000);

        // Update last seen times every second
        setInterval(() => {
            this.updateLastSeenTimes();
        }, 1000);
        
        this.init();
    }

    initSortControls() {
        const sortSelect = document.getElementById('talkgroup-sort');
        const sortButton = document.getElementById('sort-direction');

        sortSelect.addEventListener('change', () => {
            this.sortBy = sortSelect.value;
            this.sortTalkgroups();
        });

        sortButton.addEventListener('click', () => {
            this.sortAsc = !this.sortAsc;
            sortButton.classList.toggle('asc', this.sortAsc);
            sortButton.textContent = this.sortAsc ? '↓' : '↑';
            this.sortTalkgroups();
        });
    }

    initTimeFilter() {
        const timeFilter = document.getElementById('time-filter');
        timeFilter.addEventListener('change', () => {
            this.timeFilter = parseInt(timeFilter.value);
            this.sortTalkgroups();
        });
    }

    isTalkgroupVisible(talkgroup) {
        // Always show unassigned talkgroup
        if (talkgroup.data.id === 'unassigned') return true;
        
        // Show all if no time filter
        if (this.timeFilter === 0) return true;

        // Check if talkgroup has had activity within the filter window
        const now = Date.now();
        const filterTime = this.timeFilter * 60 * 1000; // Convert minutes to milliseconds
        return talkgroup.lastSeen && (now - talkgroup.lastSeen) <= filterTime;
    }

    sortTalkgroups() {
        const talkgroups = Array.from(this.talkgroups.values())
            .filter(tg => this.isTalkgroupVisible(tg));
        
        talkgroups.sort((a, b) => {
            // Always put unassigned at the end
            if (a.data.id === 'unassigned') return 1;
            if (b.data.id === 'unassigned') return -1;
            
            let comparison = 0;
            switch (this.sortBy) {
                case 'id':
                    comparison = parseInt(a.data.id) - parseInt(b.data.id);
                    break;
                case 'first-seen':
                    comparison = (a.firstSeen || 0) - (b.firstSeen || 0);
                    break;
                case 'last-seen':
                    comparison = (a.lastSeen || 0) - (b.lastSeen || 0);
                    break;
                case 'unit-count':
                    comparison = a.unitCount - b.unitCount;
                    break;
                case 'event-count':
                    comparison = (a.eventCount || 0) - (b.eventCount || 0);
                    break;
            }
            return this.sortAsc ? comparison : -comparison;
        });

        // Clear container and reorder elements
        while (this.container.firstChild) {
            this.container.removeChild(this.container.firstChild);
        }
        talkgroups.forEach(talkgroup => {
            this.container.appendChild(talkgroup.element);
        });
    }

    recordTalkgroupEvent(talkgroupId) {
        const talkgroup = this.talkgroups.get(talkgroupId);
        if (!talkgroup) return;

        const now = Date.now();
        talkgroup.firstSeen = talkgroup.firstSeen || now;
        talkgroup.lastSeen = now;

        if (talkgroupId === 'unassigned') {
            // For unassigned, update unit count
            const countEl = talkgroup.element.querySelector('.unit-count');
            if (countEl) {
                countEl.textContent = `${talkgroup.unitCount} units`;
            }
        } else {
            // For normal talkgroups, update event count and last seen
            talkgroup.eventCount = (talkgroup.eventCount || 0) + 1;
            const eventCountEl = talkgroup.element.querySelector('.event-count');
            const lastSeenEl = talkgroup.element.querySelector('.last-seen');
            if (eventCountEl) {
                eventCountEl.textContent = `${talkgroup.eventCount} events`;
            }
            if (lastSeenEl) {
                const timeAgo = Math.round((now - talkgroup.lastSeen) / 1000);
                lastSeenEl.textContent = timeAgo === 0 ? 'Just now' : `${timeAgo}s ago`;
            }
        }

        this.sortTalkgroups();
    }

    updateStats() {
        // Update unit counts
        const activeUnits = this.units.size;
        // Don't count unassigned in total talkgroups
        const activeTalkgroups = Array.from(this.talkgroups.keys()).filter(id => id !== 'unassigned').length;
        
        // Count units by status
        let transmittingCount = 0;
        let emergencyCount = 0;
        this.units.forEach(unit => {
            // Only count transmitting units that aren't stale
            if (Array.from(unit.element.classList).some(c => c.startsWith('transmitting-')) && 
                !unit.element.classList.contains('stale')) {
                transmittingCount++;
            }
            if (unit.element.classList.contains('emergency')) {
                emergencyCount++;
            }
        });

        // Update display
        document.getElementById('units-count').textContent = activeUnits;
        document.getElementById('talkgroups-count').textContent = activeTalkgroups;
        document.getElementById('transmitting-count').textContent = transmittingCount;
        document.getElementById('emergency-count').textContent = emergencyCount;
    }

    updateUnitStates() {
        const now = Date.now();
        this.units.forEach(unit => {
            const timeSinceLastActivity = now - new Date(unit.data.last_seen).getTime();
            const element = unit.element;
            
            // Remove recent state after timeout
            if (timeSinceLastActivity > this.RECENT_TIMEOUT) {
                element.classList.remove('recent');
            }
            
            // Add stale state after timeout
            if (timeSinceLastActivity > this.STALE_TIMEOUT) {
                element.classList.add('stale');
            } else {
                element.classList.remove('stale');
            }
        });
    }

    async init() {
        // Initialize API configuration
        await initializeApiConfig();
        
        // Subscribe to unit events
        wsManager.subscribe([
            'unit.activity',  // Unit transmitting start/end
            'unit.status',    // Unit online/offline status
            'unit.location'   // Unit talkgroup monitoring
        ]);

        // Handle unit transmitting activity
        wsManager.on('unit.activity', async data => {
            await this.handleUnitActivity(data);
            this.statsManager.recordEvent();
        });
        
        // Handle unit online/offline status
        wsManager.on('unit.status', data => {
            this.updateUnitStatus(data.unit_id, data.status);
            this.statsManager.recordEvent();
        });

        // Handle unit talkgroup monitoring changes
        wsManager.on('unit.location', async data => {
            const unit = this.units.get(data.unit);
            if (unit) {
                unit.data.talkgroup_id = data.talkgroup;
                unit.data.talkgroup_tag = data.talkgroup_tag;
                unit.element.classList.add('location-update', 'recent');
                setTimeout(() => unit.element.classList.remove('location-update'), 2000);
                await this.addOrUpdateUnit(unit.data);
            }
            this.statsManager.recordEvent();
        });

        // Connect WebSocket
        wsManager.connect();

        // Load talkgroups first, then units
        await this.loadTalkgroups();
        await this.loadUnits();
    }

    async loadTalkgroups() {
        try {
            const response = await fetch(`${getApiBaseUrl()}/talkgroups`);
            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }
            const data = await response.json();
            if (data.data) {
                // Initialize talkgroup data map
                this.talkgroupData = new Map();
                data.data.forEach(tg => {
                    this.talkgroupData.set(parseInt(tg.talkgroup), {
                        id: parseInt(tg.talkgroup),
                        tag: tg.talkgroup_tag,
                        description: tg.description,
                        category: tg.category
                    });
                });
                this.statusManager.flash(`talkgroups.loaded (${data.count} total)`);
            }
        } catch (error) {
            console.error('Error loading talkgroups:', error);
        }
    }

    async loadUnits() {
        try {
            let hasMore = true;
            let offset = 0;
            const limit = 100;

            while (hasMore) {
                // Load units in batches
                const response = await fetch(`${getApiBaseUrl()}/units/active?window=30&limit=${limit}&offset=${offset}`);
                if (!response.ok) {
                    throw new Error(`HTTP error! status: ${response.status}`);
                }
                const data = await response.json();
                
                if (!data.data?.units) break;

                // Process each unit and its recent history
                for (const unit of data.data.units) {
                    // Fetch recent history for the unit
                    const historyResponse = await fetch(`${getApiBaseUrl()}/units/${unit.unit}/history?limit=1`);
                    if (!historyResponse.ok) continue;
                    
                    const historyData = await historyResponse.json();
                    const recentActivity = historyData.data?.history?.[0];
                    
                    // Determine initial status based on most recent activity
                    let status = unit.status.online ? 'active' : 'offline';
                    let isStale = false;
                    
                    if (recentActivity) {
                        const activityAge = Date.now() - new Date(recentActivity.timestamp).getTime();
                        const oneTimeEvents = ['location', 'join', 'ack', 'data'];
                        
                        // If recent activity is a one-time event and within stale window
                        if (oneTimeEvents.includes(recentActivity.activity_type) && activityAge < this.STALE_TIMEOUT) {
                            status = `transmitting-${recentActivity.activity_type}`;
                            isStale = activityAge > 3000; // Stale after 3 seconds for one-time events
                        }
                        // For call events
                        else if (recentActivity.activity_type === 'call' && !recentActivity.end_time) {
                            status = 'transmitting-call';
                        }
                    }
                    
                    // Add or update the unit
                    await this.addOrUpdateUnit({
                        id: unit.unit,
                        tag: unit.unit_alpha_tag,
                        status: status,
                        talkgroup_id: unit.status.current_talkgroup,
                        talkgroup_tag: unit.status.current_talkgroup_tag,
                        last_seen: unit.status.last_seen
                    });
                    
                    // If it's a stale one-time event, update the classes
                    if (isStale) {
                        const unitElement = this.units.get(unit.unit)?.element;
                        if (unitElement) {
                            unitElement.classList.add('stale');
                            unitElement.classList.remove('recent');
                        }
                    }
                }
                // Update pagination
                hasMore = data.data.pagination.has_more;
                offset += limit;
                
                // Flash status for each batch
                this.statusManager.flash(`units.loaded (${offset}/${data.data.pagination.total})`);
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
        
        // Remove class after animation completes
        setTimeout(() => {
            element.classList.remove(auraClass);
        }, 3000);
    }

    async handleUnitActivity(data) {
        // Get existing unit data to preserve talkgroup
        const existingUnit = this.units.get(data.unit)?.data || {};
        
        // Map activity type to status
        let status;
        const oneTimeEvents = ['location', 'join', 'ack', 'data'];
        
        if (data.activity_type === 'end') {
            // Keep the existing transmission type when ending
            const existingClasses = Array.from(this.units.get(data.unit)?.element.classList || []);
            const transmitType = existingClasses.find(c => c.startsWith('transmitting-'));
            status = transmitType || 'active';
        } else {
            status = `transmitting-${data.activity_type}`; // call, location, data, ackresp
            
            // For one-time events, set a timeout to revert to stale
            if (oneTimeEvents.includes(data.activity_type)) {
                const unitId = data.unit;
                setTimeout(() => {
                    const unit = this.units.get(unitId);
                    if (unit) {
                        const element = unit.element;
                        // Remove transmission class and add stale
                        element.classList.remove(`transmitting-${data.activity_type}`, 'recent');
                        element.classList.add('active', 'stale');
                    }
                }, 3000); // Revert after 3 seconds
            }
        }
        
        const unit = {
            id: data.unit,
            tag: data.unit_alpha_tag,
            status: status,
            // Always keep existing talkgroup
            talkgroup_id: existingUnit.talkgroup_id || data.talkgroup,
            talkgroup_tag: existingUnit.talkgroup_tag || data.talkgroup_tag,
            last_seen: data.status.last_seen
        };

        const unitElement = this.units.get(unit.id)?.element;
        if (unitElement) {
            if (data.activity_type === 'end') {
                unitElement.classList.add('stale');
                unitElement.classList.remove('recent');
            } else {
                unitElement.classList.remove('stale');
                unitElement.classList.add('recent');
                if (unit.talkgroup_id) {
                    this.pulseGroupAura(unit.talkgroup_id, status);
                }
            }
        }

        // Record event for talkgroup
        if (unit.talkgroup_id) {
            this.recordTalkgroupEvent(unit.talkgroup_id);
        }

        await this.addOrUpdateUnit(unit);
        this.statusManager.flash(`unit.activity.${data.activity_type}`);
        this.statsManager.recordEvent();
        this.updateStats();
    }

    async ensureTalkgroupExists(talkgroupId) {
        if (this.talkgroups.has(talkgroupId)) return this.talkgroups.get(talkgroupId);

        let talkgroup;
        if (talkgroupId === 'unassigned') {
            // Special handling for unassigned units
            talkgroup = {
                id: 'unassigned',
                tag: 'Unassigned Units',
                description: 'Units not currently assigned to a talkgroup',
                category: 'System'
            };
        } else {
            // First check our cached talkgroup data
            talkgroup = this.talkgroupData?.get(parseInt(talkgroupId));
            
            // If not in cache, try to fetch it
            if (!talkgroup && talkgroupId !== 'unassigned') {
                try {
                    const response = await fetch(`${getApiBaseUrl()}/talkgroups/${talkgroupId}`);
                    if (response.ok) {
                        const data = await response.json();
                        if (data.data) {
                            talkgroup = {
                                id: data.data.talkgroup,
                                tag: data.data.talkgroup_tag,
                                description: data.data.description,
                                category: data.data.category
                            };
                            // Add to our cache
                            if (!this.talkgroupData) this.talkgroupData = new Map();
                            this.talkgroupData.set(parseInt(talkgroupId), talkgroup);
                        }
                    }
                } catch (error) {
                    console.error('Error fetching talkgroup details:', error);
                }
            }

            // Fallback if fetch failed
            if (!talkgroup) {
                talkgroup = {
                    id: talkgroupId,
                    tag: `Talkgroup ${talkgroupId}`,
                    description: '',
                    category: ''
                };
            }
        }

        const groupElement = document.createElement('div');
        groupElement.className = 'unit-group';
        groupElement.dataset.talkgroupId = talkgroupId;
        
        if (talkgroupId === 'unassigned') {
            // For unassigned, just show count
            groupElement.innerHTML = `
                <h2>
                    ${talkgroup.tag}
                    ${talkgroup.description ? `<div class="talkgroup-description">${talkgroup.description}</div>` : ''}
                    ${talkgroup.category ? `<div class="talkgroup-category">${talkgroup.category}</div>` : ''}
                    <div class="talkgroup-stats">
                        <span class="unit-count">0 units</span>
                    </div>
                </h2>
                <div class="units-container" style="display: none;"></div>
            `;
        } else {
            // Normal talkgroup with unit dots
            groupElement.innerHTML = `
                <h2>
                    ${talkgroup.tag || `Talkgroup ${talkgroup.id}`}
                    ${talkgroup.description ? `<div class="talkgroup-description">${talkgroup.description}</div>` : ''}
                    ${talkgroup.category ? `<div class="talkgroup-category">${talkgroup.category}</div>` : ''}
                    <div class="talkgroup-stats">
                        <span class="event-count">0 events</span>
                        <span class="last-seen">Never</span>
                    </div>
                </h2>
                <div class="units-container"></div>
            `;
        }

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
        // Don't remove the unassigned talkgroup
        if (talkgroupId === 'unassigned') return;

        const talkgroup = this.talkgroups.get(talkgroupId);
        if (!talkgroup) return;

        // Check both unit count and actual DOM elements
        const hasUnits = talkgroup.unitsContainer.children.length > 0;
        if (!hasUnits || talkgroup.unitCount <= 0) {
            talkgroup.element.remove();
            this.talkgroups.delete(talkgroupId);
            this.statusManager.flash('talkgroup.removed');
        }
    }

    cleanupEmptyTalkgroups() {
        // Check all talkgroups for emptiness
        this.talkgroups.forEach((talkgroup, id) => {
            // Don't remove the unassigned talkgroup
            if (id === 'unassigned') return;
            
            const hasUnits = talkgroup.unitsContainer.children.length > 0;
            if (!hasUnits) {
                talkgroup.element.remove();
                this.talkgroups.delete(id);
                this.statusManager.flash('talkgroup.removed');
            }
        });
    }

    async addOrUpdateUnit(unit) {
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
        if (currentContainer) {
            const oldTalkgroupElement = currentContainer.closest('.unit-group');
            if (oldTalkgroupElement) {
                oldTalkgroupId = oldTalkgroupElement.dataset.talkgroupId;
            }
        }

        // Get or create target container
        // Use a single 'unassigned' talkgroup for all unassigned units
        const talkgroupId = unit.talkgroup_id || 'unassigned';
        const talkgroup = await this.ensureTalkgroupExists(talkgroupId);
        targetContainer = talkgroup.unitsContainer;

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

            // Increment count for both assigned and unassigned talkgroups
            const newTalkgroup = this.talkgroups.get(talkgroupId);
            if (newTalkgroup) {
                newTalkgroup.unitCount++;
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
        
        // Remove non-transmission status classes
        element.classList.remove('active', 'offline', 'emergency');
        
        // Only remove transmission classes if starting a new transmission
        if (status.startsWith('transmitting-')) {
            element.classList.remove(
                'transmitting-call',
                'transmitting-location',
                'transmitting-data',
                'transmitting-ackresp'
            );
        }
        
        // Add appropriate status class
        if (status.startsWith('transmitting-')) {
            // For transmission types (call, location, data, ackresp)
            element.classList.add(status, 'recent');
        } else {
            switch (status) {
                case 'active':
                case 'online':
                    element.classList.add('active', 'recent');
                    break;
                case 'emergency':
                    element.classList.add('emergency', 'recent');
                    break;
                case 'offline':
                    element.classList.add('offline', 'stale');
                    break;
                default:
                    element.classList.add('offline', 'stale');
            }
        }

        // Only flash if status actually changed
        if (oldStatus !== status) {
            this.statusManager.flash(`unit.status.${status}`);
            this.statsManager.recordEvent();
            this.updateStats();
        }
    }

    updateLastSeenTimes() {
        const now = Date.now();
        this.talkgroups.forEach((talkgroup, id) => {
            // Skip unassigned talkgroup
            if (id === 'unassigned') return;

            if (talkgroup.lastSeen) {
                const lastSeenEl = talkgroup.element.querySelector('.last-seen');
                if (lastSeenEl) {
                    const timeAgo = Math.round((now - talkgroup.lastSeen) / 1000);
                    if (timeAgo < 60) {
                        lastSeenEl.textContent = timeAgo === 0 ? 'Just now' : `${timeAgo}s ago`;
                    } else if (timeAgo < 3600) {
                        const minutes = Math.floor(timeAgo / 60);
                        lastSeenEl.textContent = `${minutes}m ago`;
                    } else {
                        const hours = Math.floor(timeAgo / 3600);
                        lastSeenEl.textContent = `${hours}h ago`;
                    }
                }
            }
        });
    }

    showUnitDetails(unit, event) {
        const details = {
            'Unit ID': unit.id,
            'Unit Tag': unit.tag || 'None',
            'Status': unit.status,
            'Talkgroup': unit.talkgroup_tag || unit.talkgroup_id || 'Default',
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
