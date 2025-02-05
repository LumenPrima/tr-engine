// Time formatting utilities
export function formatTime(timestamp) {
    if (!timestamp) return 'N/A';
    const date = new Date(timestamp);
    return date.toLocaleTimeString();
}

export function formatDuration(seconds) {
    if (!seconds) return 'N/A';
    const minutes = Math.floor(seconds / 60);
    const remainingSeconds = Math.floor(seconds % 60);
    return `${minutes}:${remainingSeconds.toString().padStart(2, '0')}`;
}

export function formatUnits(units) {
    if (!units || !Array.isArray(units)) return 'None';
    return units.join(', ');
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
    const wsPort = 3001; // WebSocket server runs on port 3001 (WS_PORT)
    const wsUrl = `${protocol}//${host}:${wsPort}`;
    
    console.log('WebSocket URL:', wsUrl);
    return wsUrl;
}

// Audio playback
export async function playAudio(callId) {
    try {
        // Try m4a format first
        const response = await fetch(`${getApiBaseUrl()}/audio/call/${callId}?format=m4a`);
        if (!response.ok) {
            // If m4a fails, try wav format
            const wavResponse = await fetch(`${getApiBaseUrl()}/audio/call/${callId}?format=wav`);
            if (!wavResponse.ok) {
                throw new Error(`HTTP error! status: ${wavResponse.status}`);
            }
            return await handleAudioResponse(wavResponse, 'audio/wav');
        }
        return await handleAudioResponse(response, 'audio/mp4');
    } catch (error) {
        console.error('Error playing audio:', error);
        alert('Error playing audio: ' + error.message);
    }
}

async function handleAudioResponse(response, mimeType) {
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const audio = new Audio(url);
    
    // Set audio properties
    audio.controls = true;
    audio.preload = 'auto';
    
    // Clean up object URL when audio loads and ends
    audio.onloadeddata = () => {
        console.log('Audio loaded successfully');
    };
    audio.onended = () => {
        URL.revokeObjectURL(url);
        console.log('Audio finished, cleaned up resources');
    };
    
    // Play the audio
    try {
        await audio.play();
    } catch (error) {
        console.error('Playback failed, retrying...', error);
        // Try playing again after a short delay
        setTimeout(() => {
            audio.play().catch(e => {
                console.error('Retry failed:', e);
                URL.revokeObjectURL(url);
            });
        }, 100);
    }
    
    return audio;
}

// Export for use in HTML
window.playAudio = playAudio;
