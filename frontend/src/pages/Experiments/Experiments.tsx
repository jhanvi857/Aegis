import React from 'react';
import { useTelemetryStore } from '../../store/useTelemetryStore';
import { FlaskConical, Play } from 'lucide-react';

export const Experiments: React.FC = () => {
  const { experiments, runExperimentScenario } = useTelemetryStore();

  return (
    <div className="p-6 space-y-6 font-sans">
      <div className="border-b border-[#26262B] pb-4">
        <div className="flex items-center space-x-2">
          <FlaskConical className="w-5 h-5 text-[#e2588a]" />
          <h1 className="text-[24px] font-semibold text-white">Experiments</h1>
        </div>
        <p className="mt-2 text-[12px] text-gray-400">
          Run a configured fault scenario against a connected service. Evaluation metrics appear only when a measured run is recorded.
        </p>
      </div>

      {experiments.length === 0 ? (
        <div className="rounded-lg border border-[#26262B] bg-[#151517] p-8 text-center">
          <p className="text-sm font-semibold text-gray-200">No experiment scenarios are configured</p>
          <p className="mt-2 text-xs text-gray-400">Connect an application and add a scenario before running an experiment.</p>
        </div>
      ) : (
        <div className="space-y-3">
          {experiments.map((experiment) => (
            <div key={experiment.id} className="flex flex-wrap items-center justify-between gap-4 rounded-lg border border-[#26262B] bg-[#151517] p-4">
              <div>
                <h2 className="text-sm font-semibold text-white">{experiment.name}</h2>
                <p className="mt-1 text-xs text-gray-400">{experiment.description}</p>
                <p className="mt-2 font-mono text-[11px] text-gray-300">{experiment.faultType} · {experiment.targetService}</p>
              </div>
              <button onClick={() => runExperimentScenario(experiment.id)} className="inline-flex items-center gap-2 rounded border border-[#840032] px-3 py-2 text-xs text-white hover:bg-[#840032]/30">
                <Play className="h-3.5 w-3.5" /> Run scenario
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};
