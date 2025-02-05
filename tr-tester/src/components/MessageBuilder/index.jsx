import React, { useState, useEffect } from 'react';
import Editor from '@monaco-editor/react';
import mqttClient from '../../utils/mqttClient';
import { getMessageTemplate, getTemplateNames, validateMessage, generateTopicFromTemplate } from '../../utils/messageStore';

function MessageBuilder() {
  const [selectedTemplate, setSelectedTemplate] = useState('');
  const [messageContent, setMessageContent] = useState('');
  const [topic, setTopic] = useState('');
  const [templates, setTemplates] = useState([]);
  const [isValid, setIsValid] = useState(true);
  const [templateVars, setTemplateVars] = useState({
    sys_num: '0',
    sys_name: 'butco',
    unit: '909248',
    talkgroup: '9179',
    status: 'on',
    lat: 39.3642,
    lon: -84.5333
  });

  useEffect(() => {
    const loadTemplates = async () => {
      // Wait a bit for templates to load
      await new Promise(resolve => setTimeout(resolve, 100));
      const names = getTemplateNames();
      console.log('Available templates:', names);
      setTemplates(names);
    };
    loadTemplates();
  }, []);

  const handleTemplateChange = (e) => {
    const templateName = e.target.value;
    setSelectedTemplate(templateName);
    
    if (templateName) {
      const template = getMessageTemplate(templateName, templateVars);
      setMessageContent(template || '');
      setTopic(generateTopicFromTemplate(templateName));
    } else {
      setMessageContent('');
      setTopic('');
    }
  };

  const handleVarChange = (name, value) => {
    setTemplateVars(prev => {
      const updated = { ...prev, [name]: value };
      if (selectedTemplate) {
        const template = getMessageTemplate(selectedTemplate, updated);
        setMessageContent(template || '');
      }
      return updated;
    });
  };

  const handleEditorChange = (value) => {
    setMessageContent(value);
    setIsValid(validateMessage(value));
  };

  const handleSend = () => {
    if (!isValid || !topic) return;

    try {
      const message = JSON.parse(messageContent);
      mqttClient.publish(topic, message);
    } catch (error) {
      console.error('Error sending message:', error);
    }
  };

  const renderTemplateVars = () => {
    if (!selectedTemplate) return null;

    return (
      <div className="grid grid-cols-2 gap-4 mb-4">
        <div>
          <label className="block text-sm font-medium text-gray-700">System Number</label>
          <input
            type="text"
            value={templateVars.sys_num}
            onChange={(e) => handleVarChange('sys_num', e.target.value)}
            className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-blue-500 focus:ring-blue-500"
          />
        </div>
        <div>
          <label className="block text-sm font-medium text-gray-700">System Name</label>
          <select
            value={templateVars.sys_name}
            onChange={(e) => handleVarChange('sys_name', e.target.value)}
            className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-blue-500 focus:ring-blue-500"
          >
            <option value="butco">Butler County</option>
            <option value="hamco">Hamilton County</option>
            <option value="warco">Warren County</option>
            <option value="monco">Montgomery County</option>
          </select>
        </div>
        <div>
          <label className="block text-sm font-medium text-gray-700">Unit ID</label>
          <input
            type="text"
            value={templateVars.unit}
            onChange={(e) => handleVarChange('unit', e.target.value)}
            className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-blue-500 focus:ring-blue-500"
          />
        </div>
        <div>
          <label className="block text-sm font-medium text-gray-700">Talkgroup</label>
          <input
            type="text"
            value={templateVars.talkgroup}
            onChange={(e) => handleVarChange('talkgroup', e.target.value)}
            className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-blue-500 focus:ring-blue-500"
          />
        </div>
        {selectedTemplate.startsWith('unit_') && (
          <>
            <div>
              <label className="block text-sm font-medium text-gray-700">Status</label>
              <select
                value={templateVars.status}
                onChange={(e) => handleVarChange('status', e.target.value)}
                className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-blue-500 focus:ring-blue-500"
              >
                <option value="on">On</option>
                <option value="off">Off</option>
                <option value="location">Location</option>
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700">Location (Lat, Lon)</label>
              <div className="grid grid-cols-2 gap-2">
                <input
                  type="number"
                  step="0.0001"
                  value={templateVars.lat}
                  onChange={(e) => handleVarChange('lat', parseFloat(e.target.value))}
                  className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-blue-500 focus:ring-blue-500"
                />
                <input
                  type="number"
                  step="0.0001"
                  value={templateVars.lon}
                  onChange={(e) => handleVarChange('lon', parseFloat(e.target.value))}
                  className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-blue-500 focus:ring-blue-500"
                />
              </div>
            </div>
          </>
        )}
      </div>
    );
  };

  return (
    <div className="bg-white shadow rounded-lg p-6">
      <h2 className="text-lg font-semibold text-gray-900 mb-4">Message Builder</h2>
      
      <div className="space-y-4">
        <div>
          <label htmlFor="template" className="block text-sm font-medium text-gray-700">
            Template
          </label>
          <select
            id="template"
            value={selectedTemplate}
            onChange={handleTemplateChange}
            className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-blue-500 focus:ring-blue-500"
          >
            <option value="">Select a template</option>
            {templates.map(template => (
              <option key={template} value={template}>
                {template}
              </option>
            ))}
          </select>
        </div>

        {renderTemplateVars()}

        <div>
          <label htmlFor="topic" className="block text-sm font-medium text-gray-700">
            Topic
          </label>
          <input
            type="text"
            id="topic"
            value={topic}
            onChange={(e) => setTopic(e.target.value)}
            className="mt-1 block w-full rounded-md border-gray-300 shadow-sm focus:border-blue-500 focus:ring-blue-500"
          />
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 mb-2">
            Message Content
          </label>
          <div className="border rounded-md overflow-hidden" style={{ height: '300px' }}>
            <Editor
              height="100%"
              defaultLanguage="json"
              value={messageContent}
              onChange={handleEditorChange}
              theme="vs-light"
              options={{
                minimap: { enabled: false },
                scrollBeyondLastLine: false,
                fontSize: 14,
                tabSize: 2,
                automaticLayout: true
              }}
            />
          </div>
        </div>

        <div className="flex justify-between items-center">
          <div className="flex items-center">
            <div className={`w-3 h-3 rounded-full ${isValid ? 'bg-green-500' : 'bg-red-500'} mr-2`}></div>
            <span className="text-sm text-gray-600">
              {isValid ? 'Valid JSON' : 'Invalid JSON'}
            </span>
          </div>
          <button
            onClick={handleSend}
            disabled={!isValid || !topic || !mqttClient.isConnected()}
            className={`btn ${
              isValid && topic && mqttClient.isConnected()
                ? 'btn-primary'
                : 'btn-secondary opacity-50 cursor-not-allowed'
            }`}
          >
            Send Message
          </button>
        </div>
      </div>
    </div>
  );
}

export default MessageBuilder;
