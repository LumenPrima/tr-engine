/**
 * Timestamp utilities for standardized handling across the application.
 * 
 * Core principles:
 * 1. Internal Processing: Use Unix timestamps (seconds since epoch)
 * 2. API Boundaries: Convert to ISO 8601 strings only when sending to external systems
 * 3. Database: Store as Unix timestamps for efficient querying
 * 4. Display: Convert to locale-specific formats only at UI layer
 */

/**
 * Convert ISO 8601 string to Unix timestamp (seconds since epoch)
 * @param {string} isoString - ISO 8601 formatted date string
 * @returns {number} Unix timestamp in seconds
 */
const fromISO = (isoString) => {
    if (!isoString) return null;
    try {
        return Math.floor(new Date(isoString).getTime() / 1000);
    } catch (err) {
        console.error('Error converting ISO string to Unix timestamp:', err);
        return null;
    }
};

/**
 * Convert Unix timestamp to ISO 8601 string (only for API responses)
 * @param {number} unixTimestamp - Unix timestamp in seconds
 * @returns {string} ISO 8601 formatted date string
 */
const toISO = (unixTimestamp) => {
    if (!unixTimestamp) return null;
    try {
        return new Date(unixTimestamp * 1000).toISOString();
    } catch (err) {
        console.error('Error converting Unix timestamp to ISO string:', err);
        return null;
    }
};

/**
 * Get current Unix timestamp in seconds
 * @returns {number} Current Unix timestamp in seconds
 */
const getCurrentUnix = () => Math.floor(Date.now() / 1000);

/**
 * Get current time as ISO 8601 string (only for API responses)
 * @returns {string} Current time as ISO 8601 string
 */
const getCurrentISO = () => new Date().toISOString();

/**
 * Format Unix timestamp for display using locale settings
 * @param {number} unixTimestamp - Unix timestamp in seconds
 * @param {Object} options - Intl.DateTimeFormat options
 * @returns {string} Locale-formatted date string
 */
const formatForDisplay = (unixTimestamp, options = {}) => {
    if (!unixTimestamp) return 'N/A';
    try {
        const date = new Date(unixTimestamp * 1000);
        return date.toLocaleString(undefined, {
            timeStyle: 'medium',
            dateStyle: 'short',
            ...options
        });
    } catch (err) {
        console.error('Error formatting timestamp for display:', err);
        return 'N/A';
    }
};

/**
 * Format duration in seconds to human-readable string
 * @param {number} seconds - Duration in seconds
 * @returns {string} Formatted duration string (e.g. "5:30")
 */
const formatDuration = (seconds) => {
    if (!seconds) return 'N/A';
    try {
        const minutes = Math.floor(seconds / 60);
        const remainingSeconds = Math.floor(seconds % 60);
        return `${minutes}:${remainingSeconds.toString().padStart(2, '0')}`;
    } catch (err) {
        console.error('Error formatting duration:', err);
        return 'N/A';
    }
};

/**
 * Validate Unix timestamp
 * @param {number} timestamp - Timestamp to validate
 * @returns {boolean} True if timestamp is valid Unix timestamp
 */
const isValidUnix = (timestamp) => {
    if (!timestamp || typeof timestamp !== 'number') return false;
    // Basic validation: timestamp should be between 1970 and 2100
    return timestamp > 0 && timestamp < 4102444800; // 4102444800 is 2100-01-01
};

/**
 * Add seconds to a Unix timestamp
 * @param {number} timestamp - Base Unix timestamp in seconds
 * @param {number} seconds - Seconds to add (can be negative)
 * @returns {number} New Unix timestamp in seconds
 */
const addSeconds = (timestamp, seconds) => {
    if (!isValidUnix(timestamp)) return null;
    return timestamp + seconds;
};

/**
 * Calculate time difference in seconds between two Unix timestamps
 * @param {number} timestamp1 - First Unix timestamp in seconds
 * @param {number} timestamp2 - Second Unix timestamp in seconds
 * @returns {number} Difference in seconds
 */
const diffSeconds = (timestamp1, timestamp2) => {
    if (!isValidUnix(timestamp1) || !isValidUnix(timestamp2)) return null;
    return Math.abs(timestamp1 - timestamp2);
};

module.exports = {
    fromISO,
    toISO,
    getCurrentUnix,
    getCurrentISO,
    formatForDisplay,
    formatDuration,
    isValidUnix,
    addSeconds,
    diffSeconds
};
