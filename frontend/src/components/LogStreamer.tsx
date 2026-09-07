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
    <div className="bg-[#0B0B0C] border border-[#26262B] rounded-[10px] text-xs overflow-hidden shadow-md">
      {/* Header controls */}
      <div className="bg-[#151517] px-4 py-2.5 border-b border-[#26262B] flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center space-x-2">
          <Terminal className="w-3.5 h-3.5 text-[#e2588a]" />
          <span className="font-sans text-[11px] font-semibold text-white uppercase tracking-[0.07em] leading-[16px]">
            TELEMETRY LOG STREAM
          </span>
          <span className="text-[10px] font-sans font-semibold tracking-[0.04em] bg-[#840032]/25 text-[#f07b9e] border border-[#840032] px-1.5 py-0.5 rounded">
            {filteredLogs.length} EVENTS
          </span>
        </div>

        <div className="flex items-center space-x-2 font-sans">
          {/* Level Filter Buttons */}
          <div className="flex bg-[#0B0B0C] border border-[#26262B] rounded-md p-0.5 space-x-0.5">
            {(['ALL', 'INFO', 'WARN', 'ERROR'] as const).map((lvl) => (
              <button
                key={lvl}
                onClick={() => setLevelFilter(lvl)}
                className={`px-2 py-0.5 rounded text-[10px] font-semibold tracking-[0.04em] uppercase ${
                  levelFilter === lvl
                    ? lvl === 'ERROR'
                      ? 'bg-[#AD2831]/20 text-[#f87171] border border-[#AD2831]/60'
                      : lvl === 'WARN'
                      ? 'bg-[#D4A017]/15 text-[#e5b533] border border-[#D4A017]/60'
                      : lvl === 'INFO'
                      ? 'bg-[#3F8E4F]/15 text-[#52b767] border border-[#3F8E4F]/60'
                      : 'bg-[#1E1E22] text-white border border-[#26262B]'
                    : 'text-gray-400 hover:text-white'
                }`}
              >
                {lvl}
              </button>
            ))}
          </div>

          {/* Search Input */}
          <div className="relative">
            <Search className="w-3.5 h-3.5 text-gray-500 absolute left-2.5 top-1.5" />
            <input
              type="text"
              placeholder="Search..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="bg-[#0B0B0C] border border-[#26262B] rounded-md pl-7 pr-2 py-1 text-[11px] font-sans text-gray-200 focus:outline-none focus:border-[#840032] w-28 sm:w-36"
            />
          </div>

          {/* Copy Button */}
          <button
            onClick={handleCopy}
            className={`px-2 py-1 font-semibold flex items-center space-x-1 border rounded-md text-[10px] tracking-[0.04em] transition-all ${
              copied
                ? 'bg-[#3F8E4F]/20 text-[#52b767] border-[#3F8E4F]'
                : 'bg-[#0B0B0C] hover:bg-[#1E1E22] text-gray-300 border-[#26262B]'
            }`}
            title="Copy logs to clipboard"
          >
            <Copy className="w-3 h-3" />
            <span>{copied ? 'COPIED' : 'COPY'}</span>
          </button>
        </div>
      </div>

      {/* Log Feed Console */}
      <div className={`p-3 overflow-y-auto ${maxHeight} space-y-1 leading-normal`}>
        {filteredLogs.length === 0 ? (
          <div className="text-gray-500 text-center py-6 font-sans text-xs">
            No telemetry log entries matching current filter criteria.
          </div>
        ) : (
          filteredLogs.map((log) => {
            const levelStyle =
              log.level === 'ERROR'
                ? 'text-[#AD2831]'
                : log.level === 'WARN'
                ? 'text-[#D4A017]'
                : 'text-[#3F8E4F]';

            return (
              <div
                key={log.id}
                className="flex items-baseline space-x-2 text-[11px] hover:bg-[#151517] px-1.5 py-0.5 rounded transition-colors"
              >
                <span className="font-mono text-[10px] font-normal text-gray-500 shrink-0">{log.timestamp}</span>
                <span className={`font-sans text-[10px] font-semibold tracking-[0.04em] uppercase shrink-0 w-12 ${levelStyle}`}>
                  [{log.level}]
                </span>
                <span className="font-mono text-[11px] font-semibold text-[#e2588a] shrink-0">
                  {log.serviceName}
                </span>
                <span className="font-sans text-[11px] font-medium text-gray-200 break-all">{log.message}</span>
              </div>
            );
          })
        )}
      </div>
    </div>
  );
};
