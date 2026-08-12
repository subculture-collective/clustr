# Frontend (Vite + React 3D)

This app renders the Reddit Cluster Map using `react-force-graph-3d`.

## Dev commands

- Install deps: `npm ci`
- Start dev server: `npm run dev`
- Build: `npm run build`
- Preview build: `npm run preview`
- Check bundle size: `npm run size`

## Bundle Size Tracking

The project includes automated bundle size tracking to prevent performance regressions.

### Bundle Size Limits

- **JS Bundle**: 500 KB (gzipped)
- **CSS Bundle**: 50 KB (gzipped)

### CI Integration

On every pull request:
1. Bundle is built and analyzed
2. Size is compared against the base branch
3. CI fails if bundle exceeds limits
4. PR comment shows size comparison with diff
5. Bundle visualization (treemap) is generated as an artifact

### Local Testing

To check bundle size locally:

```bash
npm run build
npm run size
```

To see detailed JSON output:

```bash
npm run size:json
```

### Bundle Analysis

After building, a detailed bundle analysis is available at `dist/stats.html`. Open this file in a browser to see:
- Interactive treemap of all modules
- Size breakdown by package
- Gzip and Brotli compressed sizes

### Configuration

Bundle size limits are configured in `.size-limit.json`. To adjust limits:

1. Edit `.size-limit.json`
2. Update the `limit` values
3. Commit the changes

The CI will enforce the new limits on subsequent PRs.

## Environment

- `VITE_API_URL` — Base for API calls (default `/api`). It is a Vite build-time
  value. The Docker image uses the same-origin proxy unless built with
  `--build-arg VITE_API_URL=https://api.example.test`; passing this value to the
  running Nginx container does not modify compiled assets.
- Optional render caps (client-side):
  - `VITE_MAX_RENDER_NODES`
  - `VITE_MAX_RENDER_LINKS`

The frontend fetches `${VITE_API_URL || '/api'}/graph?max_nodes=...&max_links=...` and renders the result.

### Deployment probes

Through the frontend proxy, `GET /api/health` is backend liveness and
`GET /api/ready` is fail-closed readiness. The latter requires a current schema
and published graph revision, so it is the correct staging/prod gate after
migration and publication complete.

## Mobile Support

The application is fully responsive and optimized for mobile devices:

### Responsive Breakpoints
- **Mobile**: < 768px width
- **Tablet**: 768px - 1024px width
- **Desktop**: ≥ 1024px width

### Mobile Optimizations

**Performance:**
- Reduced default node count (5,000 on mobile, 10,000 on tablet, 20,000 on desktop)
- Lower pixel ratio cap (1.5x on mobile, 2x on tablet/desktop)
- Default to MEDIUM LOD tier on mobile for better framerate
- Target: >15 FPS with 5,000 nodes

**Touch Gestures:**
- **One finger drag**: Rotate camera
- **Two finger pinch**: Zoom in/out
- **Two finger drag**: Pan camera
- **Swipe up/down** (on Sidebar header): Expand/collapse bottom sheet

**UI Layout:**
- **Sidebar**: Converts from left sidebar (desktop) to bottom sheet (mobile)
- **Inspector**: Converts from right sidebar (desktop) to bottom sheet (mobile)
- **Legend/Minimap**: Auto-repositioned above bottom sheet on mobile
- **SearchBar**: Centered at top, responsive width

### Testing on Mobile Devices

**iOS (Safari):**
```bash
# Get your local IP
ipconfig getifaddr en0  # macOS
# Or: ip addr show  # Linux

# Start dev server
npm run dev -- --host

# Access from mobile: http://<your-ip>:5173
```

**Android (Chrome):**
```bash
# Same as iOS - start with --host flag
npm run dev -- --host

# Access from mobile browser
```

**Browser DevTools:**
- Chrome DevTools: Toggle device toolbar (Cmd+Shift+M / Ctrl+Shift+M)
- Test various viewport sizes: iPhone 12, iPad, etc.
- Enable touch simulation for gesture testing

## Notes

- Ensure there is no trailing slash in `VITE_API_URL` to avoid double slashes in requests.
- When running with Docker, nginx in the frontend container proxies `/api/` to the backend API.

## Features

### Edge Bundling

The frontend includes an edge bundling feature that reduces visual clutter in dense graphs by grouping links between communities into curved bundles.

**How it works:**
1. Links between the same source and target communities are grouped together
2. Groups with ≥3 links are rendered as single curved tubes (bundles)
3. Bundle thickness scales logarithmically with the number of links
4. Bundle colors blend the colors of the connected communities

**Usage:**
- Enable community detection in the UI
- Toggle "Bundle edges" checkbox to enable/disable bundling
- Bundles update dynamically as the graph simulation runs

**Implementation:**
- Core logic: `src/rendering/EdgeBundler.ts`
- Integration: `src/components/Graph3D.tsx`
- Uses THREE.js `TubeGeometry` and `CatmullRomCurve3` for smooth curves
