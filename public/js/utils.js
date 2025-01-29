// Time formatting utilities
export function formatTime(isoString) {
    if (!isoString) return 'N/A';
    try {
        const date = new Date(isoString);
        return date.toLocaleString(undefined, {
            timeStyle: 'medium',
            dateStyle: 'short'
        });
    } catch (err) {
        console.error('Error formatting time:', err);
        return 'N/A';
    }
}

export function formatDuration(seconds) {
    if (!seconds) return 'N/A';
    try {
        const minutes = Math.floor(seconds / 60);
        const remainingSeconds = Math.floor(seconds % 60);
        return `${minutes}:${remainingSeconds.toString().padStart(2, '0')}`;
    } catch (err) {
        console.error('Error formatting duration:', err);
        return 'N/A';
    }
}

export function formatUnits(units) {
    if (!units || !Array.isArray(units)) return 'None';
    return units.join(', ');
}

// Timestamp validation
export function isValidISOString(isoString) {
    if (!isoString) return false;
    try {
        const date = new Date(isoString);
        return date.toISOString() === isoString;
    } catch {
        return false;
    }
}

// API configuration management
let apiConfig = null;

export async function initializeApiConfig() {
    try {
        console.log('Fetching API configuration...');
        const response = await fetch('/api/v1/config');
        
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        
        const data = await response.json();
        if (!data?.config?.api) {
            throw new Error('Invalid API configuration format');
        }
        
        apiConfig = data.config.api;
        console.log('API configuration loaded:', {
            port: apiConfig.port,
            base_url: apiConfig.base_url
        });
        
        return apiConfig;
    } catch (error) {
        console.error('Error fetching API config:', error);
        console.warn('Using fallback configuration');
        
        // Default fallback configuration
        apiConfig = {
            port: window.location.port || 3000,
            base_url: '/api/v1'
        };
        
        console.log('Fallback configuration:', apiConfig);
        return apiConfig;
    }
}

export function getApiBaseUrl() {
    if (!apiConfig) {
        console.warn('API config not initialized, using default base URL');
        return '/api/v1';
    }
    return apiConfig.base_url || '/api/v1';
}

export function getWsBaseUrl() {
    if (!apiConfig) {
        console.warn('API config not initialized, using default WebSocket configuration');
    }
    
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = window.location.hostname;
    const port = apiConfig?.port || window.location.port || 3000;
    const wsUrl = `${protocol}//${host}${port ? `:${port}` : ''}`;
    
    console.log('WebSocket URL:', wsUrl);
    return wsUrl;
}

// Audio playback
export async function playAudio(callId) {
    try {
        const response = await fetch(`${getApiBaseUrl()}/audio/${callId}`);
        if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);
        const blob = await response.blob();
        const url = URL.createObjectURL(blob);
        const audio = new Audio(url);
        
        // Clean up object URL after audio loads
        audio.onload = () => URL.revokeObjectURL(url);
        
        // Play the audio
        await audio.play();
    } catch (error) {
        console.error('Error playing audio:', error);
        alert('Error playing audio: ' + error.message);
    }
}

// Export for use in HTML
window.playAudio = playAudio;
