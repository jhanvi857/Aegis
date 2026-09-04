import React, { useState } from 'react';
import type { LogEntry } from '../types/telemetry';
import { Terminal, Search, Copy } from 'lucide-react';

interface LogStreamerProps {
  logs: LogEntry[];
  serviceFilter?: string;
  maxHeight?: string;
}

export const LogStreamer: React.FC<LogStreamerProps> = ({
  logs,
  serviceFilter,
  maxHeight = 'max-h-72',
}) => {
  const [levelFilter, setLevelFilter] = useState<'ALL' | 'INFO' | 'WARN' | 'ERROR'>('ALL');
  const [searchQuery, setSearchQuery] = useState('');
  const [copied, setCopied] = useState(false);

  const filteredLogs = logs.filter((log) => {
    if (serviceFilter && log.serviceId !== serviceFilter) return false;
    if (levelFilter !== 'ALL' && log.level !== levelFilter) return false;
    if (searchQuery) {
      const q = searchQuery.toLowerCase();
      return (
        log.message.toLowerCase().includes(q) ||
        log.serviceName.toLowerCase().includes(q)
      );
    }
    return true;
  });

  const handleCopy = () => {
    const text = filteredLogs.map((l) => `[${l.timestamp}] [${l.level}] [${l.serviceName}] ${l.message}`).join('\n');
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="bg-[#07090E] border border-[#1E293B] rounded-xl font-mono text-xs overflow-hidden shadow-xl">
      {/* Header controls */}
      <div className="bg-[#0F172A] px-4 py-2.5 border-b border-[#1E293B] flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center space-x-2">
          <Terminal className="w-4 h-4 text-sky-400" />
          <span className="font-bold text-white uppercase tracking-wide">
            Live Telemetry Log Stream
          </span>
          <span className="text-[10px] bg-sky-950 text-sky-300 border border-sky-700 px-2 py-0.5 rounded font-bold">
            {filteredLogs.length} EVENTS
          </span>
        </div>

        <div className="flex items-center space-x-2">
          {/* Level Filter Buttons */}
          <div className="flex bg-[#07090E] border border-[#1E293B] rounded-lg p-0.5 space-x-1">
            {(['ALL', 'INFO', 'WARN', 'ERROR'] as const).map((lvl) => (
              <button
                key={lvl}
                onClick={() => setLevelFilter(lvl)}
                className={`px-2.5 py-0.5 rounded text-[10px] font-bold ${
                  levelFilter === lvl
                    ? lvl === 'ERROR'
                      ? 'bg-rose-950 text-rose-300 border border-rose-700'
                      : lvl === 'WARN'
                      ? 'bg-amber-950 text-amber-300 border border-amber-700'
                      : 'bg-sky-950 text-sky-300 border border-sky-700'
                    : 'text-slate-400 hover:text-white'
                }`}
              >
                {lvl}
              </button>
            ))}
          </div>

          {/* Search Input */}
          <div className="relative">
            <Search className="w-3.5 h-3.5 text-slate-500 absolute left-2.5 top-2" />
            <input
              type="text"
              placeholder="Search logs..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="bg-[#07090E] border border-[#1E293B] rounded-lg pl-8 pr-2 py-1 text-[11px] text-slate-200 focus:outline-none focus:border-sky-500 w-32 sm:w-44"
            />
          </div>

          {/* Copy Button */}
          <button
            onClick={handleCopy}
            className={`px-2 py-1 font-bold flex items-center space-x-1 border rounded-lg text-[10px] transition-all ${
              copied
                ? 'bg-emerald-950 text-emerald-300 border-emerald-700'
                : 'bg-[#07090E] hover:bg-[#1E293B] text-slate-300 border-[#1E293B]'
            }`}
            title="Copy logs to clipboard"
          >
            <Copy className="w-3.5 h-3.5" />
            <span>{copied ? 'COPIED' : 'COPY'}</span>
          </button>
        </div>
      </div>

      {/* Log Feed Console */}
      <div className={`p-4 overflow-y-auto ${maxHeight} space-y-1.5 leading-relaxed`}>
        {filteredLogs.length === 0 ? (
          <div className="text-slate-500 text-center py-6">
            No telemetry log entries matching current filter criteria.
          </div>
        ) : (
          filteredLogs.map((log) => {
            const levelStyle =
              log.level === 'ERROR'
                ? 'text-rose-400 font-bold'
                : log.level === 'WARN'
                ? 'text-amber-400 font-bold'
                : 'text-sky-400 font-semibold';

            return (
              <div
                key={log.id}
                className="flex items-start space-x-2 text-[11px] font-mono hover:bg-[#0F172A] px-1.5 py-0.5 rounded transition-colors"
              >
                <span className="text-slate-500 shrink-0">{log.timestamp}</span>
                <span className={`shrink-0 w-14 ${levelStyle}`}>
                  [{log.level}]
                </span>
                <span className="text-purple-400 shrink-0 font-bold">
                  [{log.serviceName}]
                </span>
                <span className="text-slate-200 break-all">{log.message}</span>
              </div>
            );
          })
        )}
      </div>
    </div>
  );
};
