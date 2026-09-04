import React, { useState } from 'react';
import { useTelemetryStore } from '../../store/useTelemetryStore';
import { ServiceCard } from '../../components/ServiceCard';
import { ServiceDetailModal } from './ServiceDetailModal';
import { Server, Search } from 'lucide-react';

export const Services: React.FC = () => {
  const { services, selectedServiceId, setSelectedServiceId } = useTelemetryStore();
  const [filterStatus, setFilterStatus] = useState<string>('ALL');
  const [searchQuery, setSearchQuery] = useState<string>('');

  const filteredServices = services.filter((s) => {
    if (filterStatus !== 'ALL' && s.status.toUpperCase() !== filterStatus) return false;
    if (searchQuery) {
      const q = searchQuery.toLowerCase();
      return (
        s.name.toLowerCase().includes(q) ||
        s.id.toLowerCase().includes(q) ||
        s.type.toLowerCase().includes(q)
      );
    }
    return true;
  });

  return (
    <div className="p-6 space-y-6 font-mono">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-[#1F2937] pb-4">
        <div>
          <div className="flex items-center space-x-2">
            <Server className="w-5 h-5 text-sky-400" />
            <h1 className="text-xl font-bold text-gray-100 uppercase tracking-wide">
              Microservices Catalog
            </h1>
          </div>
          <p className="text-xs text-gray-400">
            Real-time health status, pod replicas, and telemetry gauges for active cluster workloads
          </p>
        </div>

        {/* Filters and Search */}
        <div className="flex items-center space-x-3">
          <div className="flex bg-[#111827] border border-[#1F2937] rounded p-0.5 space-x-1 text-xs">
            {['ALL', 'HEALTHY', 'DEGRADED', 'CRITICAL'].map((st) => (
              <button
                key={st}
                onClick={() => setFilterStatus(st)}
                className={`px-3 py-1 rounded font-bold text-[11px] ${
                  filterStatus === st
                    ? st === 'CRITICAL'
                      ? 'bg-red-950 text-red-400 border border-red-800'
                      : st === 'DEGRADED'
                      ? 'bg-amber-950 text-amber-400 border border-amber-800'
                      : 'bg-sky-950 text-sky-400 border border-sky-800'
                    : 'text-gray-400 hover:text-gray-200'
                }`}
              >
                {st}
              </button>
            ))}
          </div>

          <div className="relative">
            <Search className="w-4 h-4 text-gray-500 absolute left-2.5 top-2.5" />
            <input
              type="text"
              placeholder="Search service..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="bg-[#111827] border border-[#1F2937] rounded pl-8 pr-3 py-1.5 text-xs text-gray-200 focus:outline-none focus:border-sky-500 w-44 sm:w-60"
            />
          </div>
        </div>
      </div>

      {/* Services Grid */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        {filteredServices.map((service) => (
          <ServiceCard
            key={service.id}
            service={service}
            onSelect={(id) => setSelectedServiceId(id)}
            isSelected={selectedServiceId === service.id}
          />
        ))}
      </div>

      {/* Detailed Modal view if service selected */}
      {selectedServiceId && (
        <ServiceDetailModal
          serviceId={selectedServiceId}
          onClose={() => setSelectedServiceId(null)}
        />
      )}
    </div>
  );
};
