/**
 * ShareButton component - generates and copies shareable URL with current state
 */

import { useState } from "react";
import { generateShareURL, type AppState } from "../utils/urlState";
import { useMobileDetect } from "../hooks/useMobileDetect";

interface Props {
  getState: () => AppState;
}

export default function ShareButton({ getState }: Props) {
  const [copied, setCopied] = useState(false);
  const { isMobile } = useMobileDetect();

  const handleShare = async () => {
    try {
      const state = getState();
      const url = generateShareURL(state);
      
      if (!navigator.clipboard) {
        throw new Error("Clipboard API not available");
      }
      await navigator.clipboard.writeText(url);
      setCopied(true);
      
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error("Failed to copy URL:", err);
      // Fallback for browsers that don't support clipboard API
      alert("Failed to copy to clipboard. Please copy the URL manually from the address bar.");
    }
  };

  return (
    <div className={`fixed z-40
      ${isMobile 
        ? 'right-3 top-36'
        : 'left-[22rem] top-24'
      }`}>
      <button
        onClick={handleShare}
        className={`instrument-panel instrument-button rounded px-3 py-2 text-xs ${
          copied
            ? "text-[#78d6b0]"
            : "text-white"
        }`}
        aria-label={copied ? "Link copied to clipboard" : "Share current view - Copy link to clipboard"}
        title="Copy shareable link to clipboard"
        aria-live="polite"
      >
        <span aria-hidden="true">{copied ? "✓ " : "↗ "}</span>
        {copied ? "Copied" : "Share Link"}
      </button>
    </div>
  );
}
