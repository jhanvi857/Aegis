import React from 'react';
import {
  ResponsiveContainer,
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
} from 'recharts';

interface DataPoint {
  timestamp: string;
  [key: string]: any;
}

interface MetricChartProps {
  title: string;
  data: DataPoint[];
  dataKey: string;
  lineColor?: string;
  unit?: string;
  height?: number;
}

export const MetricChart: React.FC<MetricChartProps> = ({
  title,
  data,
  dataKey,
  lineColor = '#38BDF8',
  unit = '',
  height = 180,
}) => {
  const latestValue = data.length > 0 ? data[data.length - 1][dataKey] : 0;

  return (
    <div className="p-4 bg-[#0F172A] border border-[#1E293B] rounded-xl shadow-lg font-mono">
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center space-x-2">
          <span className="w-2.5 h-2.5 rounded-full" style={{ backgroundColor: lineColor }} />
          <h4 className="text-xs font-bold text-slate-200 uppercase tracking-wider">
            {title}
          </h4>
        </div>
        <div className="text-xs font-bold text-white">
          {latestValue}
          <span className="text-slate-400 font-normal ml-1">{unit}</span>
        </div>
      </div>

      <div style={{ width: '100%', height: height }}>
        <ResponsiveContainer width="100%" height="100%">
          <LineChart data={data} margin={{ top: 5, right: 10, left: -20, bottom: 0 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="#1E293B" vertical={false} />
            <XAxis
              dataKey="timestamp"
              stroke="#64748B"
              fontSize={10}
              tickLine={false}
              axisLine={false}
            />
            <YAxis
              stroke="#64748B"
              fontSize={10}
              tickLine={false}
              axisLine={false}
            />
            <Tooltip
              contentStyle={{
                backgroundColor: '#07090E',
                borderColor: '#1E293B',
                borderRadius: '8px',
                fontSize: '11px',
                color: '#F8FAFC',
                boxShadow: '0 10px 15px -3px rgba(0, 0, 0, 0.5)',
              }}
              itemStyle={{ color: lineColor }}
              formatter={(val: any) => [`${val} ${unit}`, title]}
            />
            <Line
              type="monotone"
              dataKey={dataKey}
              stroke={lineColor}
              strokeWidth={2.5}
              dot={false}
              isAnimationActive={false}
            />
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
};
