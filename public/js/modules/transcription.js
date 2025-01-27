// Transcription-related functionality
import { formatTime, formatDuration } from '../utils.js';

const API_BASE_URL = 'http://localhost:3002/api/v1';

// Fetch recent transcriptions for a talkgroup
export async function fetchTalkgroupTranscriptions(talkgroupId) {
    try {
        const response = await fetch(`${API_BASE_URL}/transcription/talkgroups/${talkgroupId}/recent_transcriptions?limit=10`);
        if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);
        const data = await response.json();
        
        const content = document.querySelector('#talkgroup-activity .content');
        if (!data?.transcriptions || data.transcriptions.length === 0) {
            content.innerHTML += '<div class="status-item">No recent transcriptions</div>';
            return;
        }

        content.innerHTML += `
            <div class="transcription-section">
                <h4>Recent Transcriptions</h4>
                ${data.transcriptions.map(t => `
                    <div class="status-item">
                        <div class="transcription">
                            <strong>Transcription:</strong> ${t.text}
                            <div class="transcription-meta">
                                Duration: ${formatDuration(t.audio_duration)}
                                Processing: ${t.processing_time.toFixed(2)}s
                                ${t.emergency ? '<span class="badge badge-danger">Emergency</span>' : ''}
                            </div>
                            <div class="timestamp">${formatTime(t.timestamp)}</div>
                        </div>
                    </div>
                `).join('')}
            </div>
        `;
    } catch (error) {
        console.error('Error fetching talkgroup transcriptions:', error);
    }
}

// Fetch transcription statistics
export async function fetchTranscriptionStats() {
    try {
        const response = await fetch(`${API_BASE_URL}/transcription/stats`);
        if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);
        const data = await response.json();
        
        const content = document.querySelector('#system-performance .content');
        if (!data?.stats) {
            return;
        }

        const stats = data.stats;
        content.innerHTML += `
            <div class="stat-section">
                <h4>Transcription Stats</h4>
                <div class="stat-group">
                    <div class="stat-box">
                        <div class="stat-value">${stats.total_transcriptions}</div>
                        <div class="stat-label">Total Transcriptions</div>
                    </div>
                    <div class="stat-box">
                        <div class="stat-value">${formatDuration(stats.total_duration)}</div>
                        <div class="stat-label">Total Audio Duration</div>
                    </div>
                    <div class="stat-box">
                        <div class="stat-value">${stats.avg_processing_time.toFixed(1)}s</div>
                        <div class="stat-label">Avg Processing Time</div>
                    </div>
                </div>
                <div class="stat-group">
                    <div class="stat-box">
                        <div class="stat-value">${stats.words_per_second.toFixed(1)}</div>
                        <div class="stat-label">Words/Second</div>
                    </div>
                    <div class="stat-box">
                        <div class="stat-value">${stats.total_words}</div>
                        <div class="stat-label">Total Words</div>
                    </div>
                </div>
            </div>
        `;
    } catch (error) {
        console.error('Error fetching transcription stats:', error);
    }
}
