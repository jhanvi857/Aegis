import React from 'react';
import type { LucideIcon } from 'lucide-react';

interface StatusCardProps {
  title: string;
  value: string | number;
  subValue?: string;
  icon: LucideIcon;
  statusColor?: 'emerald' | 'amber' | 'red' | 'sky' | 'purple' | 'healthy' | 'warning' | 'critical' | 'intervention' | 'execution';
  trend?: string;
  badgeText?: string;
}

export const StatusCard: React.FC<StatusCardProps> = ({
  title,
  value,
  subValue,
  icon: Icon,
  statusColor = 'healthy',
  trend,
  badgeText,
}) => {
  const colorMap = {
    healthy: {
      text: 'text-[#52b767]',
      border: 'border-[#3F8E4F]/50',
      bg: 'bg-[#3F8E4F]/15',
      iconBg: 'bg-[#3F8E4F]/20 text-[#52b767] border border-[#3F8E4F]/50',
    },
    emerald: {
      text: 'text-[#52b767]',
      border: 'border-[#3F8E4F]/50',
      bg: 'bg-[#3F8E4F]/15',
      iconBg: 'bg-[#3F8E4F]/20 text-[#52b767] border border-[#3F8E4F]/50',
    },
    warning: {
      text: 'text-[#e5b533]',
      border: 'border-[#D4A017]/50',
      bg: 'bg-[#D4A017]/15',
      iconBg: 'bg-[#D4A017]/20 text-[#e5b533] border border-[#D4A017]/50',
    },
    amber: {
      text: 'text-[#e5b533]',
      border: 'border-[#D4A017]/50',
      bg: 'bg-[#D4A017]/15',
      iconBg: 'bg-[#D4A017]/20 text-[#e5b533] border border-[#D4A017]/50',
    },
    critical: {
      text: 'text-[#f87171]',
      border: 'border-[#AD2831]/60',
      bg: 'bg-[#AD2831]/20',
      iconBg: 'bg-[#AD2831]/25 text-[#f87171] border border-[#AD2831]/60',
    },
    red: {
      text: 'text-[#f87171]',
      border: 'border-[#AD2831]/60',
      bg: 'bg-[#AD2831]/20',
      iconBg: 'bg-[#AD2831]/25 text-[#f87171] border border-[#AD2831]/60',
    },
    intervention: {
      text: 'text-[#f07b9e]',
      border: 'border-[#840032]/70',
      bg: 'bg-[#840032]/25',
      iconBg: 'bg-[#840032] text-white border border-[#a80f49]',
    },
    execution: {
      text: 'text-[#f472b6]',
      border: 'border-[#89023E]/70',
      bg: 'bg-[#89023E]/25',
      iconBg: 'bg-[#89023E] text-white border border-[#b30855]',
    },
    sky: {
      text: 'text-gray-200',
      border: 'border-[#26262B]',
      bg: 'bg-[#1E1E22]',
      iconBg: 'bg-[#0B0B0C] text-gray-300 border border-[#26262B]',
    },
    purple: {
      text: 'text-[#f07b9e]',
      border: 'border-[#840032]/70',
      bg: 'bg-[#840032]/20',
      iconBg: 'bg-[#840032] text-white border border-[#a80f49]',
    },
  };

  const theme = colorMap[statusColor] || colorMap.healthy;

  return (
    <div className="p-4 bg-[#151517] border border-[#26262B] hover:border-[#383840] rounded-lg shadow-sm font-sans transition-all">
      <div className="flex items-center justify-between mb-2.5">
        <span className="text-[10px] font-semibold text-gray-400 uppercase tracking-[0.06em] leading-[14px]">
          {title}
        </span>
        <div className={`p-1.5 rounded-md ${theme.iconBg}`}>
          <Icon className="w-3.5 h-3.5" />
        </div>
      </div>

      <div className="flex items-baseline justify-between">
        <div className="flex items-baseline space-x-2">
          <span className={`text-[28px] font-semibold tracking-[-0.03em] leading-[32px] font-mono ${theme.text}`}>
            {value}
          </span>
          {subValue && (
            <span className="text-[11px] text-gray-400 font-normal leading-[16px]">
              {subValue}
            </span>
          )}
        </div>

        {badgeText && (
          <span className={`text-[10px] px-2 py-0.5 rounded font-semibold tracking-[0.04em] leading-[16px] border uppercase font-sans ${theme.bg} ${theme.text} ${theme.border}`}>
            {badgeText}
          </span>
        )}
      </div>

      {trend && (
        <div className="mt-2.5 text-[11px] text-gray-400 flex items-center justify-between border-t border-[#26262B] pt-2">
          <span>{trend}</span>
          <span className="text-gray-400 font-mono text-[10px]">1s tick</span>
        </div>
      )}
    </div>
  );
};
