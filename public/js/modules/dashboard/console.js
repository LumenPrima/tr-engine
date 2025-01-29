// Console message management
export class ConsoleManager {
    constructor() {
        this.messageConsole = document.getElementById('message-console');
        this.transcriptionConsole = document.getElementById('transcription-console');
        this.initialStateConsole = document.getElementById('initial-state-console');
        this.maxMessages = 100;
        this.messages = [];
        this.transcriptions = [];
        this.allowedEvents = new Set(['call.start', 'unit.status', 'transcription.new']);
    }

    addMessage(type, data) {
        // Only show allowed event types
        if (!this.allowedEvents.has(type)) return;

        if (type === 'transcription.new') {
            this.addTranscription(data);
            return;
        }

        const now = new Date();
        const time = now.toLocaleTimeString();
        
        const message = document.createElement('div');
        message.className = 'console-message new';
        
        const timestamp = document.createElement('span');
        timestamp.className = 'timestamp';
        timestamp.textContent = time;
        
        const typeSpan = document.createElement('span');
        typeSpan.className = 'type';
        typeSpan.textContent = type;

        const content = document.createElement('span');
        content.className = 'content';
        
        // Extract relevant info based on event type
        if (type === 'unit.status' && (data.unit || data.id)) {
            content.textContent = ` Unit: ${data.unit || data.id}`;
        }
        if (type === 'call.start' && data.talkgroup) {
            content.textContent = ` TG: ${data.talkgroup}`;
            if (data.talkgroup_description) {
                content.textContent += ` (${data.talkgroup_description})`;
            }
        }
        if (data.emergency) {
            content.textContent += ' [EMERGENCY]';
            content.classList.add('emergency');
        }
        
        message.appendChild(timestamp);
        message.appendChild(typeSpan);
        message.appendChild(content);
        
        this.messageConsole.appendChild(message);
        this.messages.push(message);
        
        // Remove 'new' class after animation
        setTimeout(() => {
            message.classList.remove('new');
        }, 2000);
        
        // Remove old messages if we exceed maxMessages
        while (this.messages.length > this.maxMessages) {
            const oldMessage = this.messages.shift();
            oldMessage.remove();
        }
        
        // Auto-scroll to bottom-right
        this.messageConsole.scrollTop = this.messageConsole.scrollHeight;
        this.messageConsole.scrollLeft = this.messageConsole.scrollWidth;
    }

    setInitialState(data) {
        this.initialStateConsole.textContent = JSON.stringify(data, null, 2);
    }

    addTranscription(data) {
        const now = new Date();
        const time = now.toLocaleTimeString();
        
        const message = document.createElement('div');
        message.className = 'console-message new';
        
        const timestamp = document.createElement('span');
        timestamp.className = 'timestamp';
        timestamp.textContent = time;
        
        const typeSpan = document.createElement('span');
        typeSpan.className = 'type';
        typeSpan.textContent = `TG ${data.talkgroup}`;
        if (data.talkgroup_tag) {
            typeSpan.textContent += ` (${data.talkgroup_tag})`;
        }

        const content = document.createElement('span');
        content.className = 'content';
        content.textContent = ` ${data.text}`;
        
        if (data.emergency) {
            content.classList.add('emergency');
        }
        
        message.appendChild(timestamp);
        message.appendChild(typeSpan);
        message.appendChild(content);
        
        this.transcriptionConsole.appendChild(message);
        this.transcriptions.push(message);
        
        // Remove 'new' class after animation
        setTimeout(() => {
            message.classList.remove('new');
        }, 2000);
        
        // Remove old transcriptions if we exceed maxMessages
        while (this.transcriptions.length > this.maxMessages) {
            const oldMessage = this.transcriptions.shift();
            oldMessage.remove();
        }
        
        // Auto-scroll to bottom
        this.transcriptionConsole.scrollTop = this.transcriptionConsole.scrollHeight;
    }
}

export default new ConsoleManager();
