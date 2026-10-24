import { memo } from 'react';
import type { EdgeProps } from '@xyflow/react';
import {
  BaseEdge,
  EdgeLabelRenderer,
  getBezierPath,
} from '@xyflow/react';
import { Check, X, Database, HardDrive, ArrowRight, Radio } from 'lucide-react';
import type { EdgeSource, EdgeState, EdgeType } from '../../api/registry';

export interface CustomEdgeData extends Record<string, unknown> {
  edgeId?: string;
  sourceType?: EdgeSource;
  state?: EdgeState;
  edgeType?: EdgeType;
  confidence?: number;
  onAccept?: (edgeId: string) => void;
  onReject?: (edgeId: string) => void;
}

export const CustomGraphEdge = memo(({
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  style = {},
  markerEnd,
  data,
  selected,
}: EdgeProps<any>) => {
  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition,
  });

  const edgeData = (data || {}) as CustomEdgeData;
  const isSuggested = edgeData.state === 'suggested' || edgeData.sourceType === 'suggested';
  const edgeType = edgeData.edgeType || 'sync';
  const confidence = edgeData.confidence !== undefined ? Math.round(edgeData.confidence * 100) : null;

  // Custom styling based on edge type & state
  let strokeColor = '#4B5563'; // default gray
  let strokeDasharray = undefined;

  if (isSuggested) {
    strokeColor = '#C084FC'; // purple for suggestions
    strokeDasharray = '5 5';
  } else if (edgeType === 'db') {
    strokeColor = '#38BDF8'; // cyan for DB
  } else if (edgeType === 'cache') {
    strokeColor = '#34D399'; // green for cache
  } else if (edgeType === 'async') {
    strokeColor = '#FB923C'; // orange for async
    strokeDasharray = '4 4';
  } else if (selected) {
    strokeColor = '#E2588A';
  }

  const customStyle = {
    ...style,
    stroke: strokeColor,
    strokeWidth: selected ? 2.5 : isSuggested ? 2 : 1.75,
    strokeDasharray: strokeDasharray || style.strokeDasharray,
  };

  const typeIconMap = {
    db: Database,
    cache: HardDrive,
    async: Radio,
    sync: ArrowRight,
  };
  const TypeIcon = typeIconMap[edgeType] || ArrowRight;

  return (
    <>
      <BaseEdge path={edgePath} markerEnd={markerEnd} style={customStyle} />
      <EdgeLabelRenderer>
        <div
          style={{
            position: 'absolute',
            transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
            pointerEvents: 'all',
          }}
          className="nodrag nopan flex items-center space-x-1 bg-[#151517]/95 border border-[#2E2E35] rounded-full px-2 py-0.5 shadow-md text-[10px] select-none hover:border-[#840032]"
        >
          {/* Edge Type Icon */}
          <TypeIcon className="w-2.5 h-2.5 text-gray-400" />
          <span className="font-mono text-[9px] uppercase font-semibold text-gray-300">
            {edgeType}
          </span>

          {/* Source badge */}
          {isSuggested ? (
            <span className="bg-purple-900/60 text-purple-300 border border-purple-700/60 px-1.5 py-0.2 rounded text-[8px] font-bold uppercase tracking-wider">
              SUGGESTED {confidence !== null && `${confidence}%`}
            </span>
          ) : edgeData.sourceType === 'discovered' ? (
            <span className="bg-cyan-950/70 text-cyan-300 border border-cyan-800/60 px-1 py-0.2 rounded text-[8px] font-mono">
              TRACE {confidence !== null && `${confidence}%`}
            </span>
          ) : (
            <span className="text-[8px] text-gray-400 font-mono uppercase">
              MANUAL
            </span>
          )}

          {/* Inline Accept/Reject buttons for suggestions */}
          {isSuggested && edgeData.edgeId && (
            <div className="flex items-center space-x-1 pl-1 border-l border-gray-700">
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  edgeData.onAccept?.(edgeData.edgeId!);
                }}
                title="Accept suggested dependency"
                className="p-0.5 rounded bg-emerald-600/30 hover:bg-emerald-600 text-emerald-300 hover:text-white transition-colors"
              >
                <Check className="w-3 h-3" />
              </button>
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  edgeData.onReject?.(edgeData.edgeId!);
                }}
                title="Reject suggested dependency (will not reappear)"
                className="p-0.5 rounded bg-rose-600/30 hover:bg-rose-600 text-rose-300 hover:text-white transition-colors"
              >
                <X className="w-3 h-3" />
              </button>
            </div>
          )}
        </div>
      </EdgeLabelRenderer>
    </>
  );
});
