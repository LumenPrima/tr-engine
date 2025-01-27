// Utility functions
const API_BASE_URL = 'http://localhost:3002/api/v1';

// Format timestamp
function formatTime(timestamp) {
    if (!timestamp) return 'Unknown';
    const date = new Date(timestamp);
    return date.toLocaleString();
}

// Format duration in seconds to human-readable string
function formatDuration(seconds) {
    if (!seconds) return '0s';
    const minutes = Math.floor(seconds / 60);
    const remainingSeconds = Math.floor(seconds % 60);
    if (minutes === 0) return `${remainingSeconds}s`;
    return `${minutes}m ${remainingSeconds}s`;
}

// Format array of units into readable string
function formatUnits(units) {
    if (!units || units.length === 0) return 'None';
    return units.join(', ');
}

// Play audio for a call
async function playAudio(callId) {
    try {
        // Convert call ID from "sys_tg_timestamp" to "tg-timestamp" format
        const [_, tg, timestamp] = callId.split('_');
        const formattedCallId = `${tg}-${timestamp}`;

        // First check if audio exists
        const response = await fetch(`${API_BASE_URL}/audio/call/${formattedCallId}`, {
            method: 'HEAD'
        });
        
        if (!response.ok) {
            throw new Error(response.status === 404 ? 'Audio not available yet' : 'Failed to load audio');
        }

        const audioUrl = `${API_BASE_URL}/audio/call/${formattedCallId}`;
        const audio = new Audio(audioUrl);
        
        // Add error handling for audio element
        audio.onerror = () => {
            throw new Error('Failed to play audio');
        };

        await audio.play();
    } catch (error) {
        console.error('Error playing audio:', error);
        alert('Error playing audio: ' + error.message);
    }
}

// Export functions
export {
    formatTime,
    formatDuration,
    formatUnits,
    playAudio
};

// Export to window for HTML use
Object.assign(window, {
    formatTime,
    formatDuration,
    formatUnits,
    playAudio
});
