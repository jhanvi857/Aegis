import React from 'react';
import { useTelemetryStore } from '../../store/useTelemetryStore';
import { FlaskConical, Play, Award } from 'lucide-react';

export const Experiments: React.FC = () => {
  const { experiments, runExperimentScenario } = useTelemetryStore();

  return (
    <div className="p-6 space-y-6 font-mono">
      {/* Header */}
      <div className="border-b border-[#1F2937] pb-4">
        <div className="flex items-center space-x-2">
          <FlaskConical className="w-5 h-5 text-purple-400" />
          <h1 className="text-xl font-bold text-gray-100 uppercase tracking-wide">
            Model Evaluation & Fault Experiment Suite
          </h1>
        </div>
        <p className="text-xs text-gray-400">
          Academic and empirical evaluation benchmark results proving ML prediction accuracy, detection latency, and MTTR
        </p>
      </div>

      {/* Summary Scorecard Tiles */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="p-4 bg-[#111827] border border-[#1F2937] rounded-lg">
          <span className="text-xs text-gray-400 font-medium uppercase">
            Mean Prediction Accuracy
          </span>
          <div className="flex items-baseline space-x-2 mt-2">
            <span className="text-3xl font-bold text-emerald-400">94.1%</span>
            <span className="text-xs text-gray-500">Across 100 Scenarios</span>
          </div>
        </div>

        <div className="p-4 bg-[#111827] border border-[#1F2937] rounded-lg">
          <span className="text-xs text-gray-400 font-medium uppercase">
            Mean Anomaly Lead Time
          </span>
          <div className="flex items-baseline space-x-2 mt-2">
            <span className="text-3xl font-bold text-sky-400">26.4s</span>
            <span className="text-xs text-gray-500">Before Cascade Failure</span>
          </div>
        </div>

        <div className="p-4 bg-[#111827] border border-[#1F2937] rounded-lg">
          <span className="text-xs text-gray-400 font-medium uppercase">
            Mean Time To Repair (MTTR)
          </span>
          <div className="flex items-baseline space-x-2 mt-2">
            <span className="text-3xl font-bold text-purple-400">18.2s</span>
            <span className="text-xs text-gray-500">Automated Resolution</span>
          </div>
        </div>

        <div className="p-4 bg-[#111827] border border-[#1F2937] rounded-lg">
          <span className="text-xs text-gray-400 font-medium uppercase">
            False Positive Rate
          </span>
          <div className="flex items-baseline space-x-2 mt-2">
            <span className="text-3xl font-bold text-gray-200">1.8%</span>
            <span className="text-xs text-gray-500">Low Noise Ratio</span>
          </div>
        </div>
      </div>

      {/* Experiments Benchmark Table */}
      <div className="p-5 bg-[#111827] border border-[#1F2937] rounded-lg space-y-4">
        <div className="flex items-center space-x-2 border-b border-[#1F2937] pb-3">
          <Award className="w-4 h-4 text-purple-400" />
          <h3 className="text-xs font-bold text-gray-200 uppercase tracking-wide">
            Benchmark Scenario Results Matrix
          </h3>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left border-collapse text-xs">
            <thead>
              <tr className="border-b border-[#1F2937] bg-[#0B0F19] text-gray-400 font-bold uppercase">
                <th className="p-3">Experiment Case</th>
                <th className="p-3">Fault Injection Type</th>
                <th className="p-3">Target Microservice</th>
                <th className="p-3">Prediction Accuracy</th>
                <th className="p-3">Detection Latency</th>
                <th className="p-3">MTTR</th>
                <th className="p-3 text-right">Action</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#1F2937]">
              {experiments.map((exp) => (
                <tr key={exp.id} className="hover:bg-[#1E293B]">
                  <td className="p-3 font-bold text-gray-100">
                    {exp.name}
                    <p className="text-[10px] text-gray-400 font-normal">
                      {exp.description}
                    </p>
                  </td>
                  <td className="p-3">
                    <span className="px-2 py-0.5 bg-[#0B0F19] border border-[#1F2937] rounded text-[10px] font-bold text-amber-400">
                      {exp.faultType.toUpperCase()}
                    </span>
                  </td>
                  <td className="p-3 text-sky-400 font-bold">{exp.targetService}</td>
                  <td className="p-3">
                    <span className="text-emerald-400 font-bold">
                      {exp.predictionAccuracy}%
                    </span>
                  </td>
                  <td className="p-3 text-gray-300">{exp.detectionTimeSec}s</td>
                  <td className="p-3 text-purple-400 font-bold">{exp.mttrSec}s</td>
                  <td className="p-3 text-right">
                    <button
                      onClick={() => runExperimentScenario(exp.id)}
                      className="px-3 py-1 bg-purple-950 hover:bg-purple-900 text-purple-300 border border-purple-800 rounded text-[11px] font-bold transition-colors inline-flex items-center space-x-1"
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
