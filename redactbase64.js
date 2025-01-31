const fs = require('fs');
const path = require('path');

// Directory containing the MQTT message files
const baseDir = './mqtt_messages';

// Function to strip Base64 data from specific fields
function stripBase64Data(filePath) {
    // Read the file content
    let content = fs.readFileSync(filePath, 'utf8');

    // Regular expression to match Base64 fields and their values
    const base64Regex = /"audio_wav_base64":"[^"]*"|"audio_m4a_base64":"[^"]*"/g;

    // Replace Base64 data with "removed_data"
    const strippedContent = content.replace(base64Regex, (match) => {
        // Extract the field name
        const fieldName = match.split(':')[0];
        // Replace the value with "removed_data"
        return `${fieldName}:"removed_data"`;
    });

    // Write the stripped content back to the file
    fs.writeFileSync(filePath, strippedContent, 'utf8');
    console.log(`Stripped Base64 data in file: ${filePath}`);
}

// Function to process all files in the directory
function processFilesInDirectory(directory) {
    // Read all files in the directory
    const files = fs.readdirSync(directory);

    // Process each file
    files.forEach((file) => {
        const filePath = path.join(directory, file);

        // Check if it's a file (not a directory)
        if (fs.statSync(filePath).isFile()) {
            stripBase64Data(filePath);
        }
    });
}

// Start processing files in the base directory
processFilesInDirectory(baseDir);

console.log('Base64 data stripping process completed.');