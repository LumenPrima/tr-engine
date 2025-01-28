const fs = require('fs').promises;
const path = require('path');

const flattenObject = (obj, prefix = '') => {
  return Object.keys(obj).reduce((acc, k) => {
    const pre = prefix.length ? prefix + '_' : '';
    if (typeof obj[k] === 'object' && obj[k] !== null && !Array.isArray(obj[k])) {
      // Recursively flatten nested objects
      Object.assign(acc, flattenObject(obj[k], pre + k));
    } else {
      // Keep arrays and primitive types as-is
      acc[pre + k] = obj[k];
    }
    return acc;
  }, {});
};

const transformMqttMessage = (message) => {
  // Start with the original message
  const baseTransform = {
    type: message.type,
    timestamp: message.timestamp,
    instance_id: message.instance_id,
    received_at: new Date()
  };

  // Recursively flatten nested objects based on message type
  let flattenedContent = {};
  
  if (message.type === 'audio' && message.call?.metadata) {
    flattenedContent = flattenObject(message.call.metadata);
    
    // Preserve specific arrays for audio messages
    if (message.call.metadata.freqList) {
      flattenedContent.freqList = message.call.metadata.freqList;
    }
    if (message.call.metadata.srcList) {
      flattenedContent.srcList = message.call.metadata.srcList;
    }
  } else if (message[message.type]) {
    // For other message types, flatten the type-specific object
    flattenedContent = flattenObject(message[message.type]);
  }

  // Merge flattened content with base transform
  return {
    ...baseTransform,
    ...flattenedContent
  };
};

async function processDirectory(dirPath, outputBasePath) {
  // Ensure output directory exists
  const outputDir = path.join(outputBasePath, path.basename(dirPath) + '_transformed');
  await fs.mkdir(outputDir, { recursive: true });

  // Read all files in the directory
  const files = await fs.readdir(dirPath);

  // Process each JSON file
  for (const file of files) {
    if (path.extname(file).toLowerCase() === '.json') {
      try {
        // Read the original file
        const filePath = path.join(dirPath, file);
        const fileContent = await fs.readFile(filePath, 'utf8');
        
        // Parse the JSON
        const originalMessage = JSON.parse(fileContent);
        
        // Transform the message
        const transformedMessage = transformMqttMessage(originalMessage);
        
        // Write the transformed message to a new file
        const outputFilePath = path.join(outputDir, file);
        await fs.writeFile(
          outputFilePath, 
          JSON.stringify(transformedMessage, null, 2)
        );

        console.log(`Processed: ${file}`);
      } catch (error) {
        console.error(`Error processing ${file}:`, error);
      }
    }
  }
}

async function processAllDirectories(baseDir, outputBaseDir) {
  // Read all directories in the base directory
  const entries = await fs.readdir(baseDir, { withFileTypes: true });
  
  // Filter for directories that start with 'tr-mqtt'
  const mqttDirs = entries
    .filter(entry => entry.isDirectory() && entry.name.startsWith('tr-mqtt'))
    .map(entry => path.join(baseDir, entry.name));

  // Process each MQTT directory
  for (const dir of mqttDirs) {
    await processDirectory(dir, outputBaseDir);
  }
}

// Usage
const baseDir = process.argv[2] || '.';
const outputBaseDir = process.argv[3] || path.join(baseDir, 'transformed');

processAllDirectories(baseDir, outputBaseDir)
  .then(() => console.log('Transformation complete'))
  .catch(console.error);