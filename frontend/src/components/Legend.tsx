/**
 * Legend component - displays color schemes for node types and communities
 */

import type { TypeFilters } from "../types/ui";
import { useMobileDetect } from "../hooks/useMobileDetect";

interface Props {
  filters: TypeFilters;
  useCommunityColors?: boolean;
  communityCount?: number;
}

const NODE_TYPE_COLORS = [
  { key: "subreddit", label: "Subreddit", color: "#78d6b0" },
  { key: "user", label: "User", color: "#69a7d8" },
  { key: "post", label: "Post", color: "#e6bd72" },
  { key: "comment", label: "Comment", color: "#d47e96" },
] as const;

export default function Legend({ filters, useCommunityColors, communityCount }: Props) {
  const visibleTypes = NODE_TYPE_COLORS.filter((t) => filters[t.key]);
  const { isMobile } = useMobileDetect();

  return (
    <div 
      className={`instrument-panel absolute z-20 rounded-xl p-3 text-white
        ${isMobile 
          ? 'bottom-20 left-2 right-2 mx-auto max-w-xs' /* Mobile: above bottom sheet */
          : 'bottom-2 left-2' /* Desktop: bottom-left */
        }`}
      role="region"
      aria-label="Graph legend"
    >
      <div className="instrument-label mb-2"><span className="sr-only">Legend</span>Visible signals</div>
      
      {/* Node Types */}
      {!useCommunityColors && (
        <div className="space-y-1" role="list" aria-label="Node types">
          {visibleTypes.map((type) => (
            <div key={type.key} className="flex items-center gap-2 text-xs" role="listitem">
              <div
                className="w-3 h-3 rounded"
                style={{ backgroundColor: type.color }}
                aria-hidden="true"
              />
              <span>{type.label}</span>
            </div>
          ))}
        </div>
      )}

      {/* Community Colors */}
      {useCommunityColors && (
        <div className="space-y-1">
          <div className="flex items-center gap-2 text-xs">
            <div className="w-3 h-3 rounded bg-gradient-to-r from-red-500 via-blue-500 to-green-500" aria-hidden="true" />
            <span>
              {communityCount ? `${communityCount} communities` : "Communities"}
            </span>
          </div>
          <div className="mt-1 text-xs text-[#9aaba8]">
            Colors by community detection
          </div>
        </div>
      )}

      {/* Size Legend */}
      <div className="mt-3 border-t border-white/10 pt-2">
        <div className="text-xs text-[#9aaba8]">
          <span className="sr-only">Node size = degree (connections)</span>Radius = entity weight
        </div>
      </div>
    </div>
  );
}
