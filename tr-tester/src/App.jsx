import React from 'react';
import ConnectionPanel from './components/ConnectionPanel';
import MessageBuilder from './components/MessageBuilder';
import QuickActions from './components/QuickActions';
import MessageHistory from './components/MessageHistory';

function App() {
  return (
    <div className="min-h-screen bg-gray-100">
      <header className="bg-white shadow">
        <div className="max-w-7xl mx-auto py-4 px-4">
          <h1 className="text-2xl font-bold text-gray-900">TR-Engine Tester</h1>
        </div>
      </header>
      <main className="max-w-7xl mx-auto py-6 px-4">
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <div className="space-y-6">
            <ConnectionPanel />
            <QuickActions />
          </div>
          <div className="space-y-6">
            <MessageBuilder />
            <MessageHistory />
          </div>
        </div>
      </main>
    </div>
  );
}

export default App;
