import { useEffect, useState } from 'react';
import { useTelemetryStore } from './store/useTelemetryStore';
import { Navbar } from './components/Navbar';
import { Sidebar } from './components/Sidebar';
import { TimeTravelBar } from './components/TimeTravelBar';

import { Dashboard } from './pages/Dashboard/Dashboard';
import { Services } from './pages/Services/Services';
import { DependencyGraph } from './pages/DependencyGraph/DependencyGraph';
import { Predictions } from './pages/Predictions/Predictions';
import { Chaos } from './pages/Chaos/Chaos';
import { Recovery } from './pages/Recovery/Recovery';
import { Experiments } from './pages/Experiments/Experiments';
import { Settings } from './pages/Settings/Settings';

export function App() {
  const [activeTab, setActiveTab] = useState<string>('dashboard');
  const { tickTelemetry, fetchInitialData, refreshIntervalMs } = useTelemetryStore();

  // Load initial backend state on mount
  useEffect(() => {
    fetchInitialData();
  }, [fetchInitialData]);

  // Run live 1-second telemetry poll loop
  useEffect(() => {
    const interval = setInterval(() => {
      tickTelemetry();
    }, refreshIntervalMs);

    return () => clearInterval(interval);
  }, [tickTelemetry, refreshIntervalMs]);

  return (
    <div className="min-h-screen bg-[#0B0B0C] text-gray-100 font-sans flex flex-col antialiased">
      {/* Top Navbar */}
      <Navbar activeTab={activeTab} setActiveTab={setActiveTab} />

      {/* Time Travel Replay Banner if active */}
      <TimeTravelBar />

      {/* Main Body Shell */}
      <div className="flex-1 flex overflow-hidden">
        {/* Sidebar Navigation */}
        <Sidebar activeTab={activeTab} setActiveTab={setActiveTab} />

        {/* Page Content Container */}
        <main className="flex-1 overflow-y-auto bg-[#0B0B0C]">
          {activeTab === 'dashboard' && <Dashboard setActiveTab={setActiveTab} />}
          {activeTab === 'services' && <Services />}
          {activeTab === 'graph' && <DependencyGraph />}
          {activeTab === 'predictions' && <Predictions />}
          {activeTab === 'chaos' && <Chaos />}
          {activeTab === 'recovery' && <Recovery />}
          {activeTab === 'experiments' && <Experiments />}
          {activeTab === 'settings' && <Settings />}
        </main>
      </div>
    </div>
  );
}

export default App;
