// Call-related functionality
import { formatTime, formatDuration, formatUnits, isValidISOString } from '../utils.js';
import { getApiBaseUrl } from '../utils.js';

const API_BASE_URL = getApiBaseUrl();

// Fetch and display active calls
export async function fetchActiveCalls() {
    try {
        const systemFilter = document.getElementById('active-calls-system-filter').value;
        const emergencyFilter = document.getElementById('active-calls-emergency-filter').checked;
        
        let url = `${API_BASE_URL}/calls/active`;
        const params = new URLSearchParams();
        if (systemFilter) params.append('sys_name', systemFilter);
        if (emergencyFilter) params.append('emergency', 'true');
        if (params.toString()) url += '?' + params.toString();

        const response = await fetch(url);
        if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);
        const data = await response.json();
        const content = document.querySelector('#active-calls .content');
        
        if (!data.data?.calls || data.data.calls.length === 0) {
            content.innerHTML = '<div class="status-item">No active calls</div>';
            return;
        }

        content.innerHTML = data.data.calls.slice(0, 10).map(call => `
            <div class="status-item">
                <div>
                    <div>
                        System: ${call.sys_name || 'Unknown'}
                        ${call.emergency ? '<span class="badge badge-danger">Emergency</span>' : ''}
                    </div>
                    <div>TG: ${call.talkgroup || 'Unknown'} - ${call.talkgroup_tag || 'Unknown'}</div>
                    <div>Units: ${formatUnits(call.units)}</div>
                    <div class="timestamp">Started: ${formatTime(call.start_time)}</div>
                </div>
            </div>
        `).join('');

        // Update count in header
        const header = document.querySelector('#active-calls h2');
        header.innerHTML = `Active Calls <span class="count">${data.data.calls.length}</span>`;
    } catch (error) {
        document.querySelector('#active-calls .content').innerHTML = 
            `<div class="error">Error loading active calls: ${error.message}</div>`;
    }
}

// Fetch and display recent call history
export async function fetchRecentHistory() {
    try {
        const systemFilter = document.getElementById('history-system-filter').value;
        const emergencyFilter = document.getElementById('history-emergency-filter').checked;
        
        let url = `${API_BASE_URL}/calls?limit=10`;
        const params = new URLSearchParams();
        if (systemFilter) params.append('sys_name', systemFilter);
        if (emergencyFilter) params.append('emergency', 'true');
        if (params.toString()) url += '&' + params.toString();

        const response = await fetch(url);
        if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);
        const data = await response.json();
        const content = document.querySelector('#recent-history .content');
        
        if (!data.data?.calls || data.data.calls.length === 0) {
            content.innerHTML = '<div class="status-item">No recent history</div>';
            return;
        }

        // Fetch transcriptions for each call
        const callsWithTranscriptions = await Promise.all(data.data.calls.map(async call => {
            try {
                // Use ISO timestamp for call ID construction
                const timestamp = call.start_time || call.timestamp;
                const unixTime = new Date(timestamp).getTime() / 1000;
                const callId = call.sys_num ? 
                    `${call.sys_num}_${call.talkgroup}_${unixTime}` : 
                    `${call.talkgroup}_${unixTime}`;
                
                const transcriptionResponse = await fetch(`${API_BASE_URL}/transcription/${callId}`);
                if (transcriptionResponse.ok) {
                    const transcriptionData = await transcriptionResponse.json();
                    if (transcriptionData.status === 'success') {
                        call.transcription = transcriptionData.data;
                    }
                }
            } catch (error) {
                console.error('Error fetching transcription:', error);
            }
            return call;
        }));

        content.innerHTML = callsWithTranscriptions.map(call => `
            <div class="status-item">
                <div>
                    <div>
                        System: ${call.sys_name || 'Unknown'}
                        ${call.emergency ? '<span class="badge badge-danger">Emergency</span>' : ''}
                    </div>
                    <div>TG: ${call.talkgroup || 'Unknown'} - ${call.talkgroup_tag || 'Unknown'}</div>
                    <div>Units: ${formatUnits(call.units)}</div>
                    <div>Duration: ${formatDuration(call.duration)}</div>
                    <div class="timestamp">${formatTime(call.start_time)}</div>
                    ${call.transcription ? `
                        <div class="transcription">
                            <strong>Transcription:</strong> ${call.transcription.text}
                            <div class="transcription-meta">
                                Duration: ${formatDuration(call.transcription.metadata.audio_duration)}
                                Processing: ${call.transcription.metadata.processing_time.toFixed(2)}s
                            </div>
                        </div>
                    ` : ''}
                </div>
                <div class="audio-controls">
                    <button class="button button-success" data-call-id="${call.call_id}" onclick="playAudio('${call.call_id}')">
                        Play Audio
                    </button>
                </div>
            </div>
        `).join('');
    } catch (error) {
        document.querySelector('#recent-history .content').innerHTML = 
            `<div class="error">Error loading recent history: ${error.message}</div>`;
    }
}

// Fetch and display recent calls with transcriptions
export async function fetchRecentCalls() {
    try {
        const systemFilter = document.getElementById('recent-calls-system-filter').value;
        const showTranscriptions = document.getElementById('recent-calls-transcription-filter').checked;
        
        let url = `${API_BASE_URL}/calls?limit=10`;
        const params = new URLSearchParams();
        if (systemFilter) params.append('sys_name', systemFilter);
        if (params.toString()) url += '&' + params.toString();

        const response = await fetch(url);
        if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);
        const data = await response.json();
        const content = document.querySelector('#recent-calls .content');
        
        if (!data.data?.calls || data.data.calls.length === 0) {
            content.innerHTML = '<div class="status-item">No recent calls</div>';
            return;
        }

        // Fetch transcriptions if enabled
        const calls = showTranscriptions ? 
            await Promise.all(data.data.calls.map(async call => {
                try {
                    // Use ISO timestamp for call ID construction
                    const timestamp = call.start_time || call.timestamp;
                    const unixTime = new Date(timestamp).getTime() / 1000;
                    const callId = call.sys_num ? 
                        `${call.sys_num}_${call.talkgroup}_${unixTime}` : 
                        `${call.talkgroup}_${unixTime}`;
                    
                    const transcriptionResponse = await fetch(`${API_BASE_URL}/transcription/${callId}`);
                    if (transcriptionResponse.ok) {
                        const transcriptionData = await transcriptionResponse.json();
                        if (transcriptionData.status === 'success') {
                            call.transcription = transcriptionData.data;
                        }
                    }
                } catch (error) {
                    console.error('Error fetching transcription:', error);
                }
                return call;
            })) : 
            data.data.calls;

        // Sort calls by timestamp in descending order
        const sortedCalls = [...calls].sort((a, b) => {
            const timeA = new Date(a.timestamp || a.start_time);
            const timeB = new Date(b.timestamp || b.start_time);
            return timeB - timeA;
        });

        content.innerHTML = sortedCalls.map(call => `
            <div class="status-item">
                <div>
                    <div>
                        System: ${call.sys_name || 'Unknown'}
                        ${call.emergency ? '<span class="badge badge-danger">Emergency</span>' : ''}
                    </div>
                    <div>TG: ${call.talkgroup || 'Unknown'} - ${call.talkgroup_tag || 'Unknown'}</div>
                    <div>Units: ${formatUnits(call.units)}</div>
                    <div>Duration: ${formatDuration(call.duration)}</div>
                    <div class="timestamp">${formatTime(call.start_time || call.timestamp)}</div>
                    ${showTranscriptions ? 
                        (call.transcription ? `
                            <div class="transcription">
                                <strong>Transcription:</strong> ${call.transcription.text}
                                <div class="transcription-meta">
                                    Duration: ${formatDuration(call.transcription.metadata.audio_duration)}
                                    Processing: ${call.transcription.metadata.processing_time.toFixed(2)}s
                                </div>
                            </div>
                        ` : '<div class="transcription">Transcription not available</div>')
                    : ''}
                </div>
                <div class="audio-controls">
                    <button class="button button-success" data-call-id="${call.call_id}" onclick="playAudio('${call.call_id}')">
                        Play Audio
                    </button>
                </div>
            </div>
        `).join('');

        // Update count in header
        const header = document.querySelector('#recent-calls h2');
        header.innerHTML = `Recent Calls <span class="count">${sortedCalls.length}</span>`;
    } catch (error) {
        document.querySelector('#recent-calls .content').innerHTML = 
            `<div class="error">Error loading recent calls: ${error.message}</div>`;
    }
}

// Export functions
window.fetchActiveCalls = fetchActiveCalls;
window.fetchRecentHistory = fetchRecentHistory;
window.fetchRecentCalls = fetchRecentCalls;
