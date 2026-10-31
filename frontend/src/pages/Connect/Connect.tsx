import React, { useEffect, useState } from 'react';
import { Copy, PlugZap } from 'lucide-react';
import { registryApi, type ConnectConfig } from '../../api/registry';

export const Connect: React.FC = () => {
  const [config, setConfig] = useState<ConnectConfig | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    registryApi.getConnectConfig().then(setConfig).catch((err) => setError(err.message));
  }, []);

  const copy = (value: string) => void navigator.clipboard?.writeText(value);
  const otel = config ? [
    `OTEL_EXPORTER_OTLP_ENDPOINT=${config.otlpHttpEndpoint}`,
    'OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf',
    'OTEL_SERVICE_NAME=<any-name>',
    'OTEL_TRACES_EXPORTER=otlp',
    'OTEL_METRICS_EXPORTER=otlp',
    'OTEL_METRIC_EXPORT_INTERVAL=10000',
    'OTEL_RESOURCE_ATTRIBUTES=service.namespace=<optional>,deployment.environment=<optional>',
  ].join('\n') : '';

  return (
    <div className="p-6 space-y-6 max-w-5xl">
      <div className="flex items-center gap-3 border-b border-[#26262B] pb-4">
        <PlugZap className="w-6 h-6 text-[#e2588a]" />
        <div><h1 className="text-xl font-bold text-white">Connect your app</h1>
          <p className="text-xs text-gray-400">No registration is required. Push OTLP or expose Prometheus metrics.</p></div>
      </div>
      {error && <p className="text-sm text-red-300">{error}</p>}
      {config && <div className="space-y-4">
        <Snippet title="OpenTelemetry environment" value={otel} onCopy={copy} />
        <Snippet title="Prometheus scrape target" value={config.scrapeTargetExample} onCopy={copy} />
        <Snippet title="Docker Compose" value={config.dockerComposeSnippet} onCopy={copy} />
        <Snippet title="Kubernetes annotations" value={Object.entries(config.kubernetesAnnotations).map(([k, v]) => `${k}: "${v}"`).join('\n')} onCopy={copy} />
      </div>}
    </div>
  );
};

const Snippet: React.FC<{ title: string; value: string; onCopy: (value: string) => void }> = ({ title, value, onCopy }) => (
  <section className="bg-[#151517] border border-[#26262B] rounded-lg p-4">
    <div className="flex justify-between items-center mb-2"><h2 className="text-sm font-semibold text-white">{title}</h2>
      <button onClick={() => onCopy(value)} className="text-gray-400 hover:text-white" title="Copy"><Copy className="w-4 h-4" /></button>
    </div>
    <pre className="text-xs text-gray-300 whitespace-pre-wrap overflow-x-auto">{value}</pre>
  </section>
);
