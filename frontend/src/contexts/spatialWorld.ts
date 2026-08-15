import { createContext, useContext, useRef } from 'react';
import { SpatialSceneClient } from '../data/SpatialSceneClient';

export const SpatialWorldContext = createContext<SpatialSceneClient | null>(null);
/** Standalone renders get a component-scoped client; the app provides one page-scoped client. */
export function useSpatialWorldClient(): SpatialSceneClient {
  const provided = useContext(SpatialWorldContext);
  const fallback = useRef<SpatialSceneClient | null>(null);
  if (!fallback.current) fallback.current = new SpatialSceneClient();
  return provided ?? fallback.current;
}
