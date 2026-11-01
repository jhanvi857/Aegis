import React, { useState } from 'react';
import { appsApi, type AppService } from '../../api/apps';

const emptyService = (): AppService => ({ id: '', name: '', mode: 'recommend', allowed_actions: [] });

export const Onboarding: React.FC = () => {
  const [name, setName] = useState('');
  const [services, setServices] = useState<AppService[]>([emptyService()]);
  const [topologyYaml, setTopologyYaml] = useState('');
  const [apiKey, setApiKey] = useState('');
  const [message, setMessage] = useState('');
  const update = (index: number, patch: Partial<AppService>) =>
    setServices((current) => current.map((service, i) => (i === index ? { ...service, ...patch } : service)));
  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    try {
      const result = await appsApi.create({ name, services, topology_yaml: topologyYaml || undefined });
      setApiKey(result.api_key);
      setMessage(result.warning);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'Registration failed');
    }
  };
  return (
    <div className="p-6 max-w-5xl space-y-5">
      <div><h1 className="text-2xl font-semibold text-white">Application onboarding</h1><p className="text-sm text-gray-400">Register distributed services. Default permission is recommend-only.</p></div>
      <form onSubmit={submit} className="space-y-4">
        <input required value={name} onChange={(e) => setName(e.target.value)} placeholder="Application name" className="w-full rounded bg-[#151517] border border-[#26262B] p-2 text-white" />
        {services.map((service, index) => (
          <div key={index} className="grid grid-cols-2 gap-2 rounded border border-[#26262B] p-3">
            <input required value={service.id} onChange={(e) => update(index, { id: e.target.value })} placeholder="service id" className="rounded bg-[#0B0B0C] p-2 text-white" />
            <input required value={service.name} onChange={(e) => update(index, { name: e.target.value })} placeholder="service name" className="rounded bg-[#0B0B0C] p-2 text-white" />
            <input value={service.base_url || ''} onChange={(e) => update(index, { base_url: e.target.value })} placeholder="https://service.example.com" className="rounded bg-[#0B0B0C] p-2 text-white" />
            <input value={service.health_url || ''} onChange={(e) => update(index, { health_url: e.target.value })} placeholder="/health" className="rounded bg-[#0B0B0C] p-2 text-white" />
            <input value={service.environment || ''} onChange={(e) => update(index, { environment: e.target.value })} placeholder="production" className="rounded bg-[#0B0B0C] p-2 text-white" />
            <input value={service.provider || ''} onChange={(e) => update(index, { provider: e.target.value })} placeholder="provider / region" className="rounded bg-[#0B0B0C] p-2 text-white" />
          </div>
        ))}
        <button type="button" onClick={() => setServices([...services, emptyService()])} className="text-sm text-pink-300">+ Add service</button>
        <textarea value={topologyYaml} onChange={(e) => setTopologyYaml(e.target.value)} placeholder="Optional existing topology.yaml" className="w-full h-28 rounded bg-[#151517] border border-[#26262B] p-2 text-white font-mono text-xs" />
        <button className="rounded bg-[#840032] px-4 py-2 text-white">Register application</button>
      </form>
      {message && <div className="rounded border border-amber-700 p-3 text-amber-200">{message}</div>}
      {apiKey && <div className="rounded border border-emerald-700 p-3"><div className="text-emerald-300 mb-2">Copy this key now; it is shown once.</div><code className="break-all text-white">{apiKey}</code></div>}
    </div>
  );
};
