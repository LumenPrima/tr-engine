import React from 'react';
import mqttClient from '../../utils/mqttClient';
import { getMessageTemplate } from '../../utils/messageStore';
import { MQTT_CONFIG } from '../../config';

function QuickActions() {
  const handleEmergencyCall = () => {
    if (!mqttClient.isConnected()) return;

    // Send emergency call start
    const callStart = JSON.parse(getMessageTemplate('tr-mqtt_main_call_start', {
      sys_name: 'butco',
      sys_num: 0,
      talkgroup: 9179,
      emergency: true
    }));
    mqttClient.publish(`${MQTT_CONFIG.topicPrefix}/calls`, callStart);

    // Send call end after 3 seconds
    setTimeout(() => {
      const callEnd = JSON.parse(getMessageTemplate('tr-mqtt_main_call_end', {
        sys_name: 'butco',
        sys_num: 0,
        talkgroup: 9179,
        elapsed: 3,
        length: 3.0
      }));
      mqttClient.publish(`${MQTT_CONFIG.topicPrefix}/calls`, callEnd);
    }, 3000);
  };

  const handleUnitStatusSequence = () => {
    if (!mqttClient.isConnected()) return;

    // Unit goes on duty
    const onDuty = JSON.parse(getMessageTemplate('tr-mqtt_units_butco_on', {
      sys_name: 'butco',
      sys_num: 0,
      unit: 909248
    }));
    mqttClient.publish(`${MQTT_CONFIG.topicPrefix}/units`, onDuty);

    // Unit goes off duty after 2 seconds
    setTimeout(() => {
      const offDuty = JSON.parse(getMessageTemplate('tr-mqtt_units_butco_off', {
        sys_name: 'butco',
        sys_num: 0,
        unit: 909248
      }));
      mqttClient.publish(`${MQTT_CONFIG.topicPrefix}/units`, offDuty);
    }, 2000);
  };

  return (
    <div className="bg-white shadow rounded-lg p-6">
      <h2 className="text-lg font-semibold text-gray-900 mb-4">Quick Actions</h2>
      
      <div className="space-y-4">
        <div>
          <button
            onClick={handleEmergencyCall}
            disabled={!mqttClient.isConnected()}
            className="w-full btn btn-primary bg-red-600 hover:bg-red-700 focus:ring-red-500 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Simulate Emergency Call
          </button>
          <p className="mt-1 text-sm text-gray-500">
            Sends an emergency call start followed by call end after 3 seconds
          </p>
        </div>

        <div>
          <button
            onClick={handleUnitStatusSequence}
            disabled={!mqttClient.isConnected()}
            className="w-full btn btn-primary disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Unit Status Sequence
          </button>
          <p className="mt-1 text-sm text-gray-500">
            Simulates a unit going on duty then off duty after 2 seconds
          </p>
        </div>
      </div>
    </div>
  );
}

export default QuickActions;
