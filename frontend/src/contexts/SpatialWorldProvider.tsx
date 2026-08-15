import { type PropsWithChildren, useRef } from 'react';
import { SpatialSceneClient } from '../data/SpatialSceneClient';
import { SpatialWorldContext } from './spatialWorld';

/** Pins one immutable published world for the lifetime of this page. */
export function SpatialWorldProvider({ children }: PropsWithChildren) {
  const clientRef = useRef<SpatialSceneClient | null>(null);
  if (!clientRef.current) clientRef.current = new SpatialSceneClient();
  return <SpatialWorldContext.Provider value={clientRef.current}>{children}</SpatialWorldContext.Provider>;
}
