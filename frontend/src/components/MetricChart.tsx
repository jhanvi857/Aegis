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
  lineColor = '#3F8E4F',
  unit = '',
  height = 180,
}) => {
  const latestValue = data.length > 0 ? data[data.length - 1][dataKey] : 0;

  return (
    <div className="p-4 bg-[#151517] border border-[#26262B] rounded-lg shadow-sm font-sans">
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center space-x-2">
          <span className="w-1.5 h-1.5 rounded-full" style={{ backgroundColor: lineColor }} />
          <h4 className="text-[11px] font-semibold text-gray-300 uppercase tracking-[0.07em] leading-[16px]">
            {title}
          </h4>
        </div>
        <div className="font-mono text-[14px] font-semibold text-white">
          {latestValue}
          <span className="font-sans text-[11px] text-gray-400 font-normal ml-1">{unit}</span>
        </div>
      </div>

      <div style={{ width: '100%', height: height }}>
        <ResponsiveContainer width="100%" height="100%">
          <LineChart data={data} margin={{ top: 5, right: 10, left: -20, bottom: 0 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="#26262B" vertical={false} />
            <XAxis
              dataKey="timestamp"
              stroke="#6B7280"
              fontSize={10}
              tickLine={false}
              axisLine={false}
              fontFamily="JetBrains Mono"
            />
            <YAxis
              stroke="#6B7280"
              fontSize={10}
              tickLine={false}
              axisLine={false}
              fontFamily="JetBrains Mono"
            />
            <Tooltip
              contentStyle={{
                backgroundColor: '#0B0B0C',
                borderColor: '#26262B',
                borderRadius: '8px',
                fontSize: '11px',
                fontFamily: 'JetBrains Mono',
                color: '#EDEDED',
                boxShadow: '0 8px 16px -2px rgba(0, 0, 0, 0.7)',
              }}
              itemStyle={{ color: lineColor }}
              formatter={(val: any) => [`${val} ${unit}`, title]}
            />
            <Line
              type="monotone"
              dataKey={dataKey}
              stroke={lineColor}
              strokeWidth={2}
              dot={false}
              isAnimationActive={false}
            />
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
};
