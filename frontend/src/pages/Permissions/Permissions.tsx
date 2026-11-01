import React, { useEffect, useState } from 'react';
import { appsApi, type Application, type AppService } from '../../api/apps';

const actions = ['RESTART', 'SCALE_UP', 'REROUTE_TRAFFIC', 'FLUSH_CACHE', 'INCREASE_POOL'];

export const Permissions: React.FC = () => {
  const [apps, setApps] = useState<Application[]>([]);
  const [selected, setSelected] = useState<Application | null>(null);
  const [error, setError] = useState('');
  useEffect(() => { appsApi.list().then(setApps).catch((e) => setError(e.message)); }, []);
  const save = async (service: AppService, mode: AppService['mode'], allowed_actions: string[]) => {
    if (!selected) return;
    try {
      await appsApi.updatePermissions(selected.id, service.id, mode, allowed_actions);
      const refreshed = await appsApi.get(selected.id); setSelected(refreshed); setApps(apps.map((app) => app.id === refreshed.id ? refreshed : app));
    } catch (e) { setError(e instanceof Error ? e.message : 'Permission update failed'); }
  };
  return <div className="p-6 space-y-5"><h1 className="text-2xl font-semibold text-white">Application permissions</h1>
    <select value={selected?.id || ''} onChange={(e) => setSelected(apps.find((app) => app.id === e.target.value) || null)} className="rounded bg-[#151517] border border-[#26262B] p-2 text-white">
      <option value="">Select application</option>{apps.map((app) => <option key={app.id} value={app.id}>{app.name}</option>)}
    </select>
    {error && <div className="text-rose-300">{error}</div>}
    {selected?.services.map((service) => <PermissionRow key={service.id} service={service} onSave={save} />)}
  </div>;
};

const PermissionRow: React.FC<{ service: AppService; onSave: (service: AppService, mode: AppService['mode'], actions: string[]) => void }> = ({ service, onSave }) => {
  const [mode, setMode] = useState(service.mode);
  const [allowed, setAllowed] = useState(service.allowed_actions);
  return <div className="rounded border border-[#26262B] p-4 space-y-3"><div className="font-semibold text-white">{service.name} <span className="text-gray-500">({service.id})</span></div>
    <select value={mode} onChange={(e) => setMode(e.target.value as AppService['mode'])} className="rounded bg-[#151517] p-2 text-white"><option value="observe">Observe</option><option value="recommend">Recommend</option><option value="execute">Execute with approval</option><option value="auto_low">Auto low-risk</option></select>
    <div className="flex flex-wrap gap-2">{actions.map((action) => <label key={action} className="text-xs text-gray-300"><input type="checkbox" checked={allowed.includes(action)} onChange={() => setAllowed(allowed.includes(action) ? allowed.filter((item) => item !== action) : [...allowed, action])} /> {action}</label>)}</div>
    <button onClick={() => onSave(service, mode, allowed)} className="rounded bg-slate-700 px-3 py-1 text-white">Save permissions</button>
  </div>;
};
