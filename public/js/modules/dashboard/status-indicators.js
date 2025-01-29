// Status indicator management
export class StatusIndicatorManager {
    constructor() {
        this.statusBar = document.getElementById('status-bar');
        this.messageTypes = new Map();
    }

    flash(type) {
        let indicator = this.messageTypes.get(type);
        if (!indicator) {
            // Create new indicator for this message type
            indicator = document.createElement('div');
            indicator.className = 'status-indicator';
            indicator.textContent = type;
            this.statusBar.appendChild(indicator);
            this.messageTypes.set(type, indicator);
        }
        // Flash the indicator
        indicator.classList.remove('flash');
        void indicator.offsetWidth; // Force reflow
        indicator.classList.add('flash');
    }
}

export default new StatusIndicatorManager();
