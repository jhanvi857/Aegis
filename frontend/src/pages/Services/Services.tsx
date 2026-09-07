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
    <div className="p-6 space-y-6 font-sans">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-[#26262B] pb-4">
        <div>
          <div className="flex items-center space-x-2">
            <Server className="w-5 h-5 text-[#e2588a]" />
            <h1 className="text-[24px] font-semibold text-white tracking-[-0.025em] leading-[32px]">
              Microservices Workload Catalog
            </h1>
          </div>
          <p className="text-[12px] text-gray-400 font-normal leading-[18px]">
            Real-time health status, pod replicas, and telemetry gauges for active cluster workloads
          </p>
        </div>

        {/* Filters and Search */}
        <div className="flex items-center space-x-3">
          <div className="flex bg-[#151517] border border-[#26262B] rounded-md p-0.5 space-x-0.5 text-xs">
            {['ALL', 'HEALTHY', 'DEGRADED', 'CRITICAL'].map((st) => (
              <button
                key={st}
                onClick={() => setFilterStatus(st)}
                className={`px-2.5 py-1 rounded text-[10px] font-semibold tracking-[0.04em] uppercase transition-colors ${
                  filterStatus === st
                    ? st === 'CRITICAL'
                      ? 'bg-[#AD2831]/20 text-[#f87171] border border-[#AD2831]/60'
                      : st === 'DEGRADED'
                      ? 'bg-[#D4A017]/15 text-[#e5b533] border border-[#D4A017]/60'
                      : st === 'HEALTHY'
                      ? 'bg-[#3F8E4F]/15 text-[#52b767] border border-[#3F8E4F]/60'
                      : 'bg-[#840032] text-white border border-[#840032]'
                    : 'text-gray-400 hover:text-gray-200'
                }`}
              >
                {st === 'DEGRADED' ? 'WARNING' : st}
              </button>
            ))}
          </div>

          <div className="relative">
            <Search className="w-3.5 h-3.5 text-gray-500 absolute left-2.5 top-2" />
            <input
              type="text"
              placeholder="Search services..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="bg-[#151517] border border-[#26262B] rounded-md pl-7 pr-3 py-1.5 text-xs text-gray-200 focus:outline-none focus:border-[#840032] w-40 sm:w-56 font-sans"
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
