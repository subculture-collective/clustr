/**
 * URL State Management Utilities
 * Handles synchronization of app state with URL query parameters for deep linking
 */

import type { TypeFilters } from "../types/ui";

export interface CameraPosition {
  x: number;
  y: number;
  z: number;
}

export interface AppState {
  viewMode?: "3d" | "2d" | "dashboard" | "communities" | "catalog" | "admin";
  filters?: TypeFilters;
  minDegree?: number;
  maxDegree?: number;
  camera3d?: CameraPosition;
  camera2d?: { x: number; y: number; zoom: number };
  useCommunityColors?: boolean;
  usePrecomputedLayout?: boolean;
  sizeAttenuation?: boolean;
  enableAdaptiveLOD?: boolean;
}

/**
 * Read application state from URL query parameters
 */
export function readStateFromURL(): AppState {
  if (typeof window === "undefined") return {};
  
  const params = new URLSearchParams(window.location.search);
  const state: AppState = {};

  // View mode
  const viewMode = params.get("view");
  if (viewMode === "3d" || viewMode === "2d" || viewMode === "dashboard" || viewMode === "communities" || viewMode === "catalog" || viewMode === "admin") {
    state.viewMode = viewMode;
  }
  if (window.location.pathname?.startsWith("/catalog/")) state.viewMode = "catalog";

  // Filters
  const filterSr = params.get("f_subreddit");
  const filterUser = params.get("f_user");
  const filterPost = params.get("f_post");
  const filterComment = params.get("f_comment");
  
  if (filterSr !== null || filterUser !== null || filterPost !== null || filterComment !== null) {
    state.filters = {
      subreddit: filterSr === "1",
      user: filterUser === "1",
      post: filterPost === "1",
      comment: filterComment === "1",
    };
  }

  // Degree thresholds
  const minDegree = params.get("minDegree");
  if (minDegree !== null) {
    const val = parseInt(minDegree, 10);
    if (!isNaN(val) && val >= 0) {
      state.minDegree = val;
    }
  }

  const maxDegree = params.get("maxDegree");
  if (maxDegree !== null) {
    const val = parseInt(maxDegree, 10);
    if (!isNaN(val) && val >= 0) {
      state.maxDegree = val;
    }
  }

  // 3D Camera position
  const cam3dX = params.get("cam3d_x");
  const cam3dY = params.get("cam3d_y");
  const cam3dZ = params.get("cam3d_z");
  if (cam3dX !== null && cam3dY !== null && cam3dZ !== null) {
    const x = parseFloat(cam3dX);
    const y = parseFloat(cam3dY);
    const z = parseFloat(cam3dZ);
    if (!isNaN(x) && !isNaN(y) && !isNaN(z)) {
      state.camera3d = { x, y, z };
    }
  }

  // 2D Camera position
  const cam2dX = params.get("cam2d_x");
  const cam2dY = params.get("cam2d_y");
  const cam2dZoom = params.get("cam2d_zoom");
  if (cam2dX !== null && cam2dY !== null && cam2dZoom !== null) {
    const x = parseFloat(cam2dX);
    const y = parseFloat(cam2dY);
    const zoom = parseFloat(cam2dZoom);
    if (!isNaN(x) && !isNaN(y) && !isNaN(zoom)) {
      state.camera2d = { x, y, zoom };
    }
  }

  // Community colors
  const useCommunityColors = params.get("communityColors");
  if (useCommunityColors !== null) {
    state.useCommunityColors = useCommunityColors === "1";
  }

  // Precomputed layout
  const usePrecomputedLayout = params.get("precomputedLayout");
  if (usePrecomputedLayout !== null) {
    state.usePrecomputedLayout = usePrecomputedLayout === "1";
  }

  // Size attenuation
  const sizeAttenuation = params.get("sizeAttenuation");
  if (sizeAttenuation !== null) {
    state.sizeAttenuation = sizeAttenuation === "1";
  }

  // Adaptive LOD
  const enableAdaptiveLOD = params.get("adaptiveLOD");
  if (enableAdaptiveLOD !== null) {
    state.enableAdaptiveLOD = enableAdaptiveLOD === "1";
  }

  return state;
}

/**
 * Write application state to URL query parameters (without page reload)
 */
export function writeStateToURL(state: AppState): void {
  if (typeof window === "undefined") return;

  const params = new URLSearchParams(window.location.search);

  // View mode
  if (state.viewMode !== undefined) {
    params.set("view", state.viewMode);
  }

  // Filters
  if (state.filters !== undefined) {
    params.set("f_subreddit", state.filters.subreddit ? "1" : "0");
    params.set("f_user", state.filters.user ? "1" : "0");
    params.set("f_post", state.filters.post ? "1" : "0");
    params.set("f_comment", state.filters.comment ? "1" : "0");
  }

  // Degree thresholds
  if (state.minDegree !== undefined && state.minDegree >= 0) {
    params.set("minDegree", state.minDegree.toString());
  } else {
    params.delete("minDegree");
  }

  if (state.maxDegree !== undefined && state.maxDegree >= 0) {
    params.set("maxDegree", state.maxDegree.toString());
  } else {
    params.delete("maxDegree");
  }

  // 3D Camera
  if (state.camera3d !== undefined) {
    params.set("cam3d_x", state.camera3d.x.toFixed(2));
    params.set("cam3d_y", state.camera3d.y.toFixed(2));
    params.set("cam3d_z", state.camera3d.z.toFixed(2));
  } else {
    params.delete("cam3d_x");
    params.delete("cam3d_y");
    params.delete("cam3d_z");
  }

  // 2D Camera
  if (state.camera2d !== undefined) {
    params.set("cam2d_x", state.camera2d.x.toFixed(2));
    params.set("cam2d_y", state.camera2d.y.toFixed(2));
    params.set("cam2d_zoom", state.camera2d.zoom.toFixed(2));
  } else {
    params.delete("cam2d_x");
    params.delete("cam2d_y");
    params.delete("cam2d_zoom");
  }

  // Community colors
  if (state.useCommunityColors !== undefined) {
    params.set("communityColors", state.useCommunityColors ? "1" : "0");
  }

  // Precomputed layout
  if (state.usePrecomputedLayout !== undefined) {
    params.set("precomputedLayout", state.usePrecomputedLayout ? "1" : "0");
  }

  // Size attenuation
  if (state.sizeAttenuation !== undefined) {
    params.set("sizeAttenuation", state.sizeAttenuation ? "1" : "0");
  }

  // Adaptive LOD
  if (state.enableAdaptiveLOD !== undefined) {
    params.set("adaptiveLOD", state.enableAdaptiveLOD ? "1" : "0");
  }

  // Update URL without page reload
  const newUrl = `${window.location.pathname}?${params.toString()}`;
  window.history.replaceState({}, "", newUrl);
}

/**
 * Generate a shareable URL with current state
 */
export function generateShareURL(state: AppState): string {
  if (typeof window === "undefined") return "";

  const params = new URLSearchParams();

  if (state.viewMode !== undefined) {
    params.set("view", state.viewMode);
  }

  if (state.filters !== undefined) {
    params.set("f_subreddit", state.filters.subreddit ? "1" : "0");
    params.set("f_user", state.filters.user ? "1" : "0");
    params.set("f_post", state.filters.post ? "1" : "0");
    params.set("f_comment", state.filters.comment ? "1" : "0");
  }

  if (state.minDegree !== undefined && state.minDegree >= 0) {
    params.set("minDegree", state.minDegree.toString());
  }

  if (state.maxDegree !== undefined && state.maxDegree >= 0) {
    params.set("maxDegree", state.maxDegree.toString());
  }

  if (state.camera3d !== undefined) {
    params.set("cam3d_x", state.camera3d.x.toFixed(2));
    params.set("cam3d_y", state.camera3d.y.toFixed(2));
    params.set("cam3d_z", state.camera3d.z.toFixed(2));
  }

  if (state.camera2d !== undefined) {
    params.set("cam2d_x", state.camera2d.x.toFixed(2));
    params.set("cam2d_y", state.camera2d.y.toFixed(2));
    params.set("cam2d_zoom", state.camera2d.zoom.toFixed(2));
  }

  if (state.useCommunityColors !== undefined) {
    params.set("communityColors", state.useCommunityColors ? "1" : "0");
  }

  if (state.usePrecomputedLayout !== undefined) {
    params.set("precomputedLayout", state.usePrecomputedLayout ? "1" : "0");
  }

  if (state.sizeAttenuation !== undefined) {
    params.set("sizeAttenuation", state.sizeAttenuation ? "1" : "0");
  }

  if (state.enableAdaptiveLOD !== undefined) {
    params.set("adaptiveLOD", state.enableAdaptiveLOD ? "1" : "0");
  }

  return `${window.location.origin}${window.location.pathname}?${params.toString()}`;
}
