const mqtt = require('mqtt');
const fs = require('fs');
const path = require('path');

// MQTT broker connection details
const brokerUrl = 'mqtt://trunkdash.luxprimatech.com'; // Replace with your broker URL
const clientId = 'mqtt-subscriber';

// Directory to store messages
const baseDir = './mqtt_messages';

// Ensure the base directory exists
if (!fs.existsSync(baseDir)) {
    fs.mkdirSync(baseDir);
}

// Object to store the last 5 messages for each topic
const messageStore = {};

// Connect to the MQTT broker
const client = mqtt.connect(brokerUrl, { clientId });

client.on('connect', () => {
    console.log('Connected to MQTT broker');
    
    // Subscribe to all topics using the wildcard '#'
    client.subscribe('#', (err) => {
        if (err) {
            console.error('Failed to subscribe to topics:', err);
        } else {
            console.log('Subscribed to all topics');
        }
    });
});

client.on('message', (topic, message) => {
    console.log(`Received message on topic: ${topic}`);

    // Initialize the message array for the topic if it doesn't exist
    if (!messageStore[topic]) {
        messageStore[topic] = [];
    }

    // Add the new message to the array
    messageStore[topic].push(message.toString());

    // Keep only the last 5 messages for the topic
    if (messageStore[topic].length > 5) {
        messageStore[topic].shift();
    }

    // Save the messages to a file
    saveMessagesToFile(topic);

    // Display topics that have been seen but do not yet have 5 messages
    displayIncompleteTopics();
});

client.on('error', (err) => {
    console.error('MQTT error:', err);
});

client.on('close', () => {
    console.log('Disconnected from MQTT broker');
});

function saveMessagesToFile(topic) {
    // Create a safe filename from the topic
    const safeTopic = topic.replace(/\//g, '_'); // Replace slashes with underscores
    const filePath = path.join(baseDir, `${safeTopic}.txt`);

    // Write the messages to the file
    fs.writeFileSync(filePath, messageStore[topic].join('\n'), 'utf8');
    console.log(`Saved messages for topic: ${topic}`);
}

function displayIncompleteTopics() {
    console.log('Topics with fewer than 5 messages:');
    let hasIncompleteTopics = false;

    for (const topic in messageStore) {
        if (messageStore[topic].length < 5) {
            console.log(`- ${topic}: ${messageStore[topic].length} messages`);
            hasIncompleteTopics = true;
        }
    }

    if (!hasIncompleteTopics) {
        console.log('All topics have 5 messages.');
    }
}

// Handle script termination
process.on('SIGINT', () => {
    console.log('Disconnecting from MQTT broker...');
    client.end();
    process.exit();
});