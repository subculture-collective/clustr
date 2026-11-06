import { useEffect, useState } from 'react';

interface Props {
  nodeId: string | null;
  nodeName?: string;
  nodeType?: string;
  mouseX: number;
  mouseY: number;
}

/**
 * NodeTooltip - Displays information about a hovered node
 * 
 * Appears near the cursor when hovering over a node.
 * Shows node name and type with minimal delay (<50ms target).
 */
export default function NodeTooltip({ nodeId, nodeName, nodeType, mouseX, mouseY }: Props) {
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    if (nodeId) {
      // Show tooltip immediately when node is hovered
      setVisible(true);
    } else {
      setVisible(false);
    }
  }, [nodeId]);

  if (!visible || !nodeId) {
    return null;
  }

  // Position tooltip near cursor with offset to avoid covering the node
  const offset = 15;
  const style = {
    left: `${mouseX + offset}px`,
    top: `${mouseY + offset}px`,
  };

  return (
    <div
      className="fixed z-30 bg-black/90 text-white px-3 py-2 rounded-sm shadow-lg text-sm pointer-events-none"
      style={style}
    >
      <div className="font-semibold">{nodeName || nodeId}</div>
      {nodeType && (
        <div className="text-xs opacity-75 mt-0.5">
          Type: {nodeType}
        </div>
      )}
    </div>
  );
}
