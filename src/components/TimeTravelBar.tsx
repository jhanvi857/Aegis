import React from 'react';
import { useTelemetryStore } from '../store/useTelemetryStore';
import { History, X } from 'lucide-react';

export const TimeTravelBar: React.FC = () => {
  const {
    isTimeTravelActive,
    setTimeTravelActive,
    timeTravelIndex,
    setTimeTravelIndex,
    historySnapshots,
  } = useTelemetryStore();

  if (!isTimeTravelActive) return null;

  const maxIndex = Math.max(0, historySnapshots.length - 1);
  const currentSnapshot = historySnapshots[timeTravelIndex] || historySnapshots[maxIndex];

  return (
    <div className="bg-[#0F172A] border-b border-purple-800/80 px-6 py-3 font-mono flex items-center justify-between shadow-2xl sticky top-16 z-30">
      <div className="flex items-center space-x-3">
        <div className="p-1.5 bg-purple-950 text-purple-300 border border-purple-700 rounded-lg">
          <History className="w-4 h-4" />
        </div>
        <div>
          <div className="flex items-center space-x-2">
            <span className="font-bold text-purple-300 text-xs uppercase tracking-wider">
              HISTORICAL TELEMETRY REPLAY MODE
            </span>
            <span className="text-[10px] bg-purple-950 text-purple-300 border border-purple-700 px-2 py-0.5 rounded font-bold">
              PAUSED LIVE TICK
            </span>
          </div>
          <p className="text-[11px] text-slate-400">
            Replaying snapshot at timestamp:{' '}
            <span className="text-white font-bold">
              {currentSnapshot?.timestamp || 'Now'}
            </span>
          </p>
        </div>
      </div>

      {/* Scrub Slider */}
      <div className="flex-1 max-w-xl mx-6 flex items-center space-x-3">
        <span className="text-[10px] text-slate-400 font-bold">
          {historySnapshots[0]?.timestamp || '-10m'}
        </span>
        <input
          type="range"
          min={0}
          max={maxIndex}
          value={timeTravelIndex}
          onChange={(e) => setTimeTravelIndex(parseInt(e.target.value, 10))}
          className="w-full h-2 bg-[#07090E] rounded-lg appearance-none cursor-pointer accent-purple-500"
        />
        <span className="text-[10px] text-slate-400 font-bold">
          {historySnapshots[maxIndex]?.timestamp || 'Live'}
        </span>
      </div>

      {/* Close button */}
      <button
        onClick={() => setTimeTravelActive(false)}
        className="flex items-center space-x-1 px-3 py-1.5 bg-[#07090E] hover:bg-[#1E293B] text-slate-200 border border-[#1E293B] hover:border-slate-500 rounded-lg text-xs font-bold transition-all"
      >
        <span>EXIT REPLAY</span>
        <X className="w-3.5 h-3.5" />
      </button>
    </div>
  );
};
