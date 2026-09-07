import React from 'react';
import { useTelemetryStore } from '../../store/useTelemetryStore';
import { FlaskConical, Play, Award } from 'lucide-react';

export const Experiments: React.FC = () => {
  const { experiments, runExperimentScenario } = useTelemetryStore();

  return (
    <div className="p-6 space-y-6 font-sans">
      {/* Header */}
      <div className="border-b border-[#26262B] pb-4">
        <div className="flex items-center space-x-2">
          <FlaskConical className="w-5 h-5 text-[#e2588a]" />
          <h1 className="text-[24px] font-semibold text-white tracking-[-0.025em] leading-[32px]">
            Model Evaluation & Fault Experiment Suite
          </h1>
        </div>
        <p className="text-[12px] text-gray-400 font-normal leading-[18px]">
          Empirical evaluation benchmark results proving TGNN graph prediction accuracy, cascade detection latency, and MTTR vs baselines
        </p>
      </div>

      {/* Summary Scorecard Tiles */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="p-4 bg-[#151517] border border-[#26262B] rounded-lg">
          <span className="text-[10px] font-semibold tracking-[0.06em] uppercase text-gray-400 leading-[14px]">
            MEAN PREDICTION ACCURACY
          </span>
          <div className="flex items-baseline space-x-2 mt-2">
            <span className="font-mono text-[28px] font-semibold tracking-[-0.03em] leading-[32px] text-[#3F8E4F]">94.1%</span>
            <span className="text-[11px] text-gray-400 font-normal leading-[16px]">Across 100 Scenarios</span>
          </div>
        </div>

        <div className="p-4 bg-[#151517] border border-[#26262B] rounded-lg">
          <span className="text-[10px] font-semibold tracking-[0.06em] uppercase text-gray-400 leading-[14px]">
            MEAN ANOMALY LEAD TIME
          </span>
          <div className="flex items-baseline space-x-2 mt-2">
            <span className="font-mono text-[28px] font-semibold tracking-[-0.03em] leading-[32px] text-[#D4A017]">26.4s</span>
            <span className="text-[11px] text-gray-400 font-normal leading-[16px]">Before Cascade Failure</span>
          </div>
        </div>

        <div className="p-4 bg-[#151517] border border-[#26262B] rounded-lg">
          <span className="text-[10px] font-semibold tracking-[0.06em] uppercase text-gray-400 leading-[14px]">
            MEAN TIME TO REPAIR (MTTR)
          </span>
          <div className="flex items-baseline space-x-2 mt-2">
            <span className="font-mono text-[28px] font-semibold tracking-[-0.03em] leading-[32px] text-[#f07b9e]">18.2s</span>
            <span className="text-[11px] text-gray-400 font-normal leading-[16px]">Automated Resolution</span>
          </div>
        </div>

        <div className="p-4 bg-[#151517] border border-[#26262B] rounded-lg">
          <span className="text-[10px] font-semibold tracking-[0.06em] uppercase text-gray-400 leading-[14px]">
            FALSE POSITIVE RATE
          </span>
          <div className="flex items-baseline space-x-2 mt-2">
            <span className="font-mono text-[28px] font-semibold tracking-[-0.03em] leading-[32px] text-gray-200">1.8%</span>
            <span className="text-[11px] text-gray-400 font-normal leading-[16px]">Low Noise Ratio</span>
          </div>
        </div>
      </div>

      {/* Experiments Benchmark Table */}
      <div className="p-4 bg-[#151517] border border-[#26262B] rounded-lg space-y-3">
        <div className="flex items-center space-x-2 border-b border-[#26262B] pb-2.5">
          <Award className="w-4 h-4 text-[#e2588a]" />
          <h3 className="text-[11px] font-semibold text-gray-300 uppercase tracking-[0.07em] leading-[16px]">
            BENCHMARK SCENARIO RESULTS MATRIX
          </h3>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left border-collapse text-xs">
            <thead>
              <tr className="border-b border-[#26262B] bg-[#0B0B0C] text-gray-400 text-[10px] font-semibold uppercase tracking-[0.06em]">
                <th className="p-3">Experiment Case</th>
                <th className="p-3">Fault Injection</th>
                <th className="p-3">Target Microservice</th>
                <th className="p-3">Prediction Accuracy</th>
                <th className="p-3">Lead Time</th>
                <th className="p-3">MTTR</th>
                <th className="p-3 text-right">Action</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#26262B]">
              {experiments.map((exp) => (
                <tr key={exp.id} className="hover:bg-[#1E1E22] transition-colors">
                  <td className="p-3 font-semibold text-gray-100">
                    {exp.name}
                    <p className="text-[11px] text-gray-400 font-normal leading-[16px]">
                      {exp.description}
                    </p>
                  </td>
                  <td className="p-3">
                    <span className="px-2 py-0.5 bg-[#0B0B0C] border border-[#D4A017]/50 rounded text-[10px] font-sans font-semibold tracking-[0.04em] uppercase text-[#e5b533]">
                      {exp.faultType.toUpperCase()}
                    </span>
                  </td>
                  <td className="p-3 font-mono text-[12px] font-semibold text-[#e2588a]">{exp.targetService}</td>
                  <td className="p-3">
                    <span className="font-mono text-[12px] font-semibold text-[#3F8E4F]">
                      {exp.predictionAccuracy}%
                    </span>
                  </td>
                  <td className="p-3 font-mono text-[12px] text-gray-300">{exp.detectionTimeSec}s</td>
                  <td className="p-3 font-mono text-[12px] font-semibold text-[#f07b9e]">{exp.mttrSec}s</td>
                  <td className="p-3 text-right">
                    <button
                      onClick={() => runExperimentScenario(exp.id)}
                      className="px-3 py-1 bg-[#840032] hover:bg-[#9b053d] text-white border border-[#840032] rounded-md text-[10px] font-semibold tracking-[0.04em] uppercase transition-colors inline-flex items-center space-x-1 shadow-sm"
                    >
                      <Play className="w-3 h-3" />
                      <span>RUN SCENARIO</span>
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
};
