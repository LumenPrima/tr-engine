import React, { useState, useEffect } from 'react';
import mqttClient from '../../utils/mqttClient';

function MessageHistory() {
  const [messages, setMessages] = useState([]);

  useEffect(() => {
    const handleMessage = (topic, message) => {
      setMessages(prev => [{
        timestamp: new Date(),
        topic,
        message,
        status: 'sent'
      }, ...prev].slice(0, 50)); // Keep last 50 messages
    };

    mqttClient.onMessage(handleMessage);
  }, []);

  return (
    <div className="bg-white shadow rounded-lg p-6">
      <h2 className="text-lg font-semibold text-gray-900 mb-4">Message History</h2>
      
      <div className="space-y-4">
        {messages.length === 0 ? (
          <p className="text-sm text-gray-500">No messages sent yet</p>
        ) : (
          <div className="overflow-auto max-h-[300px]">
            {messages.map((msg, index) => (
              <div key={index} className="border-b border-gray-200 py-2 last:border-0">
                <div className="flex justify-between items-start">
                  <div className="flex-1">
                    <p className="text-sm font-medium text-gray-900">
                      {msg.topic}
                    </p>
                    <p className="text-xs text-gray-500">
                      {msg.timestamp.toLocaleTimeString()}
                    </p>
                  </div>
                  <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-green-100 text-green-800">
                    {msg.status}
                  </span>
                </div>
                <pre className="mt-1 text-xs text-gray-600 overflow-auto">
                  {JSON.stringify(JSON.parse(msg.message), null, 2)}
                </pre>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

export default MessageHistory;
