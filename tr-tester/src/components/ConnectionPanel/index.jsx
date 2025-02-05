import React, { useState, useEffect } from 'react';
import mqttClient from '../../utils/mqttClient';

function ConnectionPanel() {
  const [connectionSettings, setConnectionSettings] = useState({
    host: 'localhost',
    port: '1883',
    username: '',
    password: '',
  });
  const [isConnected, setIsConnected] = useState(mqttClient.isConnected());

  useEffect(() => {
    const onConnect = () => setIsConnected(true);
    const onDisconnect = () => setIsConnected(false);

    mqttClient.onConnect(onConnect);
    mqttClient.onDisconnect(onDisconnect);

    return () => {
      mqttClient.onConnect(onConnect);
      mqttClient.onDisconnect(onDisconnect);
    };
  }, []);

  const handleConnect = () => {
    if (isConnected) {
      mqttClient.disconnect();
    } else {
      mqttClient.connect(connectionSettings);
    }
  };

  const handleChange = (e) => {
    const { name, value } = e.target;
    setConnectionSettings(prev => ({
      ...prev,
      [name]: value
    }));
  };

  return (
    <div className="bg-white shadow rounded-lg p-6">
      <h2 className="text-lg font-semibold text-gray-900 mb-4">MQTT Connection</h2>
      <div className="space-y-4">
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label htmlFor="host" className="block text-sm font-medium text-gray-700">Host</label>
            <input
              type="text"
              id="host"
              name="host"
              value={connectionSettings.host}
              onChange={handleChange}
              className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-blue-500 focus:ring-blue-500"
            />
          </div>
          <div>
            <label htmlFor="port" className="block text-sm font-medium text-gray-700">Port</label>
            <input
              type="text"
              id="port"
              name="port"
              value={connectionSettings.port}
              onChange={handleChange}
              className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-blue-500 focus:ring-blue-500"
            />
          </div>
        </div>
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label htmlFor="username" className="block text-sm font-medium text-gray-700">Username</label>
            <input
              type="text"
              id="username"
              name="username"
              value={connectionSettings.username}
              onChange={handleChange}
              className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-blue-500 focus:ring-blue-500"
            />
          </div>
          <div>
            <label htmlFor="password" className="block text-sm font-medium text-gray-700">Password</label>
            <input
              type="password"
              id="password"
              name="password"
              value={connectionSettings.password}
              onChange={handleChange}
              className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-blue-500 focus:ring-blue-500"
            />
          </div>
        </div>
        <div className="flex justify-between items-center">
          <div className="flex items-center">
            <div className={`w-3 h-3 rounded-full ${isConnected ? 'bg-green-500' : 'bg-red-500'} mr-2`}></div>
            <span className="text-sm text-gray-600">{isConnected ? 'Connected' : 'Disconnected'}</span>
          </div>
          <button
            onClick={handleConnect}
            className={`btn ${isConnected ? 'btn-secondary' : 'btn-primary'}`}
          >
            {isConnected ? 'Disconnect' : 'Connect'}
          </button>
        </div>
      </div>
    </div>
  );
}

export default ConnectionPanel;
