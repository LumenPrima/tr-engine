/**
 * Utility functions for consistent timestamp handling across the application.
 * 
 * Storage: Unix timestamps (seconds since epoch)
 * API: ISO 8601 strings
 * Display: Locale-specific formatting
 */

/**
 * Convert ISO 8601 string to Unix timestamp (seconds since epoch)
 * @param {string} isoString - ISO 8601 formatted date string
 * @returns {number} Unix timestamp in seconds
 */
const toUnix = (isoString) => {
    if (!isoString) return null;
    try {
        return Math.floor(new Date(isoString).getTime() / 1000);
    } catch (err) {
        console.error('Error converting ISO string to Unix timestamp:', err);
        return null;
    }
};

/**
 * Convert Unix timestamp to ISO 8601 string
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
 * Format timestamp for display using locale settings
 * @param {string} isoString - ISO 8601 formatted date string
 * @param {Object} options - Intl.DateTimeFormat options
 * @returns {string} Locale-formatted date string
 */
const formatForDisplay = (isoString, options = {}) => {
    if (!isoString) return 'N/A';
    try {
        const date = new Date(isoString);
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
 * @returns {string} Formatted duration string (e.g., "5:30")
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
 * Get current time as ISO 8601 string
 * @returns {string} Current time as ISO 8601 string
 */
const getCurrentTimeISO = () => new Date().toISOString();

/**
 * Get current time as Unix timestamp
 * @returns {number} Current time as Unix timestamp in seconds
 */
const getCurrentTimeUnix = () => Math.floor(Date.now() / 1000);

/**
 * Validate ISO 8601 string format
 * @param {string} isoString - String to validate
 * @returns {boolean} True if string is valid ISO 8601
 */
const isValidISO = (isoString) => {
    if (!isoString) return false;
    try {
        const date = new Date(isoString);
        return date.toISOString() === isoString;
    } catch {
        return false;
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

module.exports = {
    toUnix,
    toISO,
    formatForDisplay,
    formatDuration,
    getCurrentTimeISO,
    getCurrentTimeUnix,
    isValidISO,
    isValidUnix
};
