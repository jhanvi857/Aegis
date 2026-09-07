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
    <div className="bg-[#151517] border-b border-[#89023E] px-6 py-2.5 font-sans flex items-center justify-between shadow-lg sticky top-16 z-30">
      <div className="flex items-center space-x-3">
        <div className="p-1.5 bg-[#89023E] text-white border border-[#b30855] rounded-md">
          <History className="w-4 h-4" />
        </div>
        <div>
          <div className="flex items-center space-x-2">
            <span className="font-semibold text-[#f07b9e] text-[11px] uppercase tracking-[0.07em] leading-[16px]">
              HISTORICAL TELEMETRY REPLAY
            </span>
            <span className="text-[10px] bg-[#840032] text-white px-1.5 py-0.5 rounded font-semibold tracking-[0.04em] uppercase">
              PAUSED
            </span>
          </div>
          <p className="text-[11px] text-gray-400 font-normal leading-[16px]">
            Snapshot timestamp:{' '}
            <span className="font-mono text-white font-medium">
              {currentSnapshot?.timestamp || 'Now'}
            </span>
          </p>
        </div>
      </div>

      {/* Scrub Slider */}
      <div className="flex-1 max-w-xl mx-6 flex items-center space-x-3">
        <span className="text-[10px] text-gray-400 font-mono">
          {historySnapshots[0]?.timestamp || '-10m'}
        </span>
        <input
          type="range"
          min={0}
          max={maxIndex}
          value={timeTravelIndex}
          onChange={(e) => setTimeTravelIndex(parseInt(e.target.value, 10))}
          className="w-full h-1.5 bg-[#0B0B0C] rounded-lg appearance-none cursor-pointer accent-[#89023E]"
        />
        <span className="text-[10px] text-gray-400 font-mono">
          {historySnapshots[maxIndex]?.timestamp || 'Live'}
        </span>
      </div>

      {/* Close button */}
      <button
        onClick={() => setTimeTravelActive(false)}
        className="flex items-center space-x-1 px-3 py-1.5 bg-[#0B0B0C] hover:bg-[#1E1E22] text-gray-200 border border-[#26262B] hover:border-gray-500 rounded-md text-xs font-semibold transition-all"
      >
        <span>EXIT</span>
        <X className="w-3.5 h-3.5" />
      </button>
    </div>
  );
};
