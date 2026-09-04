import React from 'react';
import type { LucideIcon } from 'lucide-react';

interface StatusCardProps {
  title: string;
  value: string | number;
  subValue?: string;
  icon: LucideIcon;
  statusColor?: 'emerald' | 'amber' | 'red' | 'sky' | 'purple';
  trend?: string;
  badgeText?: string;
}

export const StatusCard: React.FC<StatusCardProps> = ({
  title,
  value,
  subValue,
  icon: Icon,
  statusColor = 'emerald',
  trend,
  badgeText,
}) => {
  const colorMap = {
    emerald: {
      text: 'text-emerald-400',
      border: 'border-emerald-700/60',
      bg: 'bg-emerald-950/40',
      iconBg: 'bg-emerald-950 text-emerald-400 border border-emerald-700',
    },
    amber: {
      text: 'text-amber-400',
      border: 'border-amber-700/60',
      bg: 'bg-amber-950/40',
      iconBg: 'bg-amber-950 text-amber-400 border border-amber-700',
    },
    red: {
      text: 'text-rose-400',
      border: 'border-rose-700/60',
      bg: 'bg-rose-950/40',
      iconBg: 'bg-rose-950 text-rose-400 border border-rose-700',
    },
    sky: {
      text: 'text-sky-400',
      border: 'border-sky-700/60',
      bg: 'bg-sky-950/40',
      iconBg: 'bg-sky-950 text-sky-400 border border-sky-700',
    },
    purple: {
      text: 'text-purple-400',
      border: 'border-purple-700/60',
      bg: 'bg-purple-950/40',
      iconBg: 'bg-purple-950 text-purple-400 border border-purple-700',
    },
  };

  const theme = colorMap[statusColor];

  return (
    <div className="p-4 bg-[#0F172A] border border-[#1E293B] hover:border-slate-500 rounded-xl shadow-lg font-mono transition-all">
      <div className="flex items-center justify-between mb-3">
        <span className="text-xs text-slate-400 font-bold uppercase tracking-wider">
          {title}
        </span>
        <div className={`p-2 rounded-lg ${theme.iconBg}`}>
          <Icon className="w-4 h-4" />
        </div>
      </div>

      <div className="flex items-baseline justify-between">
        <div className="flex items-baseline space-x-2">
          <span className={`text-2xl font-bold ${theme.text}`}>
            {value}
          </span>
          {subValue && (
            <span className="text-xs text-slate-500 font-normal">
              {subValue}
            </span>
          )}
        </div>

        {badgeText && (
          <span className={`text-[10px] px-2 py-0.5 rounded font-bold border uppercase ${theme.bg} ${theme.text} ${theme.border}`}>
            {badgeText}
          </span>
        )}
      </div>

      {trend && (
        <div className="mt-3 text-[11px] text-slate-400 flex items-center justify-between border-t border-[#1E293B] pt-2">
          <span>{trend}</span>
          <span className="text-slate-500 font-bold">Live 1s Tick</span>
        </div>
      )}
    </div>
  );
};
