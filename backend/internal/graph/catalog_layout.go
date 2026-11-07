package graph

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/lib/pq"
)

// catalogPlacementVersion names the rule that places subreddits in a catalog.
// A catalog built under a different version than the current one is a cold
// relayout: nothing warm-starts, every entity follows its new anchor.
const catalogPlacementVersion = "force-world-3d-v2"

const (
	defaultCatalogLayoutIterations     = 300
	defaultCatalogWarmLayoutIterations = 60
	defaultCatalogWorldRadius          = 60000.0
	// Subreddits that share no users with any other sit on a shell outside
	// the connected world, between these multiples of the world radius.
	isolatedShellInner = 1.3
	isolatedShellOuter = 1.6
	// New subreddits in a warm build start this close to their strongest
	// placed neighbour before the simulation settles them.
	warmSeedJitter = 0.02
)

// catalogLayoutResult records what the layout stage did for the catalog config.
type catalogLayoutResult struct {
	Mode     string
	Bodies   int
	Settled  int
	Isolated int
	Stats    WorldLayoutStats
}

func floatEnv(name string, fallback float64) float64 {
	if value, err := strconv.ParseFloat(os.Getenv(name), 64); err == nil && value > 0 {
		return value
	}
	return fallback
}

func boolEnv(name string) bool {
	value, err := strconv.ParseBool(os.Getenv(name))
	return err == nil && value
}

// catalogPlacementIsCold reports whether the next catalog must relayout from
// scratch: there is no current catalog, or it was placed under another rule.
func catalogPlacementIsCold(ctx context.Context, tx *sql.Tx) (bool, error) {
	var previous sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT c.config->>'placement' FROM spatial_catalogs c
JOIN spatial_catalog_current current ON current.catalog_id=c.id`).Scan(&previous)
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return !previous.Valid || previous.String != catalogPlacementVersion, nil
}

// catalogPlacementChanged reports whether the given catalog was placed under a
// different rule than the catalog of the current revision, so a revision built
// on it must move landmarks to their members instead of keeping old positions.
// With no current revision there is nothing to keep, which counts as changed.
func catalogPlacementChanged(ctx context.Context, tx *sql.Tx, catalogID int64) (bool, error) {
	var changed bool
	err := tx.QueryRowContext(ctx, `SELECT (SELECT c.config->>'placement' FROM spatial_catalogs c WHERE c.id=$1) IS DISTINCT FROM
 (SELECT pc.config->>'placement' FROM graph_revision_current current
  JOIN graph_revisions pr ON pr.id=current.revision_id
  JOIN spatial_catalogs pc ON pc.id=pr.spatial_catalog_id)`, catalogID).Scan(&changed)
	return changed, err
}

// layoutCatalogSubreddits places every subreddit at the watermark with the
// force simulation and leaves the result in the temporary table
// catalog_force_positions(subreddit_id, x, y, z, provenance).
//
// Cold: every subreddit that shares users with another is a free body; the
// rest go on the isolated shell. Warm: subreddits with a position in the
// current catalog are pinned and receive no row, new linked subreddits start
// beside their strongest placed neighbour and settle, and new isolated
// subreddits go on the shell.
func layoutCatalogSubreddits(ctx context.Context, tx *sql.Tx, watermark time.Time, cold bool, options SpatialCatalogOptions) (catalogLayoutResult, error) {
	result := catalogLayoutResult{Mode: "warm"}
	if cold {
		result.Mode = "cold"
	}
	type subreddit struct {
		id      int32
		x, y, z float64
		seeded  bool
		body    int
	}
	rows, err := tx.QueryContext(ctx, `SELECT s.id,previous.x,previous.y,previous.z
FROM subreddits s
LEFT JOIN spatial_catalog_current current ON current.singleton AND NOT $2
LEFT JOIN spatial_catalog_entities previous ON previous.catalog_id=current.catalog_id AND previous.id='subreddit_'||s.id
WHERE COALESCE(s.updated_at,s.created_at,to_timestamp(0)) <= $1 ORDER BY s.id`, watermark, cold)
	if err != nil {
		return result, fmt.Errorf("list subreddits for layout: %w", err)
	}
	var subreddits []subreddit
	index := map[int32]int{}
	for rows.Next() {
		var entry subreddit
		var x, y, z sql.NullFloat64
		if err := rows.Scan(&entry.id, &x, &y, &z); err != nil {
			rows.Close()
			return result, fmt.Errorf("scan subreddit for layout: %w", err)
		}
		if x.Valid && y.Valid && z.Valid && isFinite3D(x.Float64, y.Float64, z.Float64) {
			entry.x, entry.y, entry.z, entry.seeded = x.Float64, y.Float64, z.Float64, true
		}
		entry.body = -1
		index[entry.id] = len(subreddits)
		subreddits = append(subreddits, entry)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("read subreddits for layout: %w", err)
	}

	// Undirected overlap edges between subreddits at the watermark, in a fixed
	// order so the body indices, and therefore the layout, are reproducible.
	rows, err = tx.QueryContext(ctx, `SELECT LEAST(source_subreddit_id,target_subreddit_id),GREATEST(source_subreddit_id,target_subreddit_id),max(overlap_count)
FROM subreddit_relationships WHERE overlap_count>0 AND source_subreddit_id<>target_subreddit_id GROUP BY 1,2 ORDER BY 1,2`)
	if err != nil {
		return result, fmt.Errorf("list overlap edges for layout: %w", err)
	}
	var nodes []WorldNode
	var edges []WorldEdge
	bodyOf := func(position int) int {
		entry := &subreddits[position]
		if entry.body < 0 {
			entry.body = len(nodes)
			node := WorldNode{ID: "subreddit_" + strconv.Itoa(int(entry.id))}
			if entry.seeded {
				node.X, node.Y, node.Z, node.Seeded, node.Pinned = entry.x, entry.y, entry.z, true, true
			}
			nodes = append(nodes, node)
		}
		return entry.body
	}
	for rows.Next() {
		var source, target int32
		var weight float64
		if err := rows.Scan(&source, &target, &weight); err != nil {
			rows.Close()
			return result, fmt.Errorf("scan overlap edge for layout: %w", err)
		}
		a, okA := index[source]
		b, okB := index[target]
		if !okA || !okB {
			continue
		}
		edges = append(edges, WorldEdge{A: bodyOf(a), B: bodyOf(b), Weight: weight})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("read overlap edges for layout: %w", err)
	}
	result.Bodies = len(nodes)
	// Hubs get mass so they claim space instead of swallowing the world, and
	// each overlap is normalised by both endpoints' totals so a hub's many
	// weak links do not outweigh a niche subreddit's few strong ones.
	for _, edge := range edges {
		nodes[edge.A].Mass++
		nodes[edge.B].Mass++
	}
	for i := range nodes {
		nodes[i].Mass++
	}
	edges = PrepareWorldEdges(len(nodes), edges, 0)

	// A new body in a warm build starts next to its strongest pinned
	// neighbour; without one it starts somewhere in the world.
	if !cold {
		type seed struct {
			weight  float64
			x, y, z float64
		}
		seeds := make(map[int]seed)
		for _, edge := range edges {
			for _, pair := range [][2]int{{edge.A, edge.B}, {edge.B, edge.A}} {
				free, pinned := nodes[pair[0]], nodes[pair[1]]
				if free.Pinned || !pinned.Pinned {
					continue
				}
				if best, ok := seeds[pair[0]]; !ok || edge.Weight > best.weight {
					seeds[pair[0]] = seed{weight: edge.Weight, x: pinned.X, y: pinned.Y, z: pinned.Z}
				}
			}
		}
		for i := range nodes {
			if nodes[i].Pinned {
				continue
			}
			result.Settled++
			if best, ok := seeds[i]; ok {
				jx, jy, jz := deterministicSphericalSeed(nodes[i].ID, options.WorldRadius*warmSeedJitter)
				nodes[i].X, nodes[i].Y, nodes[i].Z = best.x+jx, best.y+jy, best.z+jz
			} else {
				nodes[i].X, nodes[i].Y, nodes[i].Z = deterministicSphericalSeed(nodes[i].ID, options.WorldRadius)
			}
			nodes[i].Seeded = true
		}
	}

	if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE catalog_force_positions(subreddit_id integer PRIMARY KEY,x float8 NOT NULL,y float8 NOT NULL,z float8 NOT NULL,provenance text NOT NULL) ON COMMIT DROP`); err != nil {
		return result, fmt.Errorf("create force position table: %w", err)
	}
	if len(nodes) > 0 && (cold || result.Settled > 0) {
		iterations := options.WarmLayoutIterations
		if cold {
			iterations = options.LayoutIterations
		}
		var positions [][3]float64
		positions, result.Stats = LayoutWorld(nodes, edges, WorldLayoutOptions{
			Iterations: iterations, TargetRadius: options.WorldRadius, Model: WorldModelForceAtlas, LinLog: true,
		})
		for i := range nodes {
			if nodes[i].Pinned {
				continue
			}
			position := index[int32(mustSubredditID(nodes[i].ID))]
			subreddits[position].x, subreddits[position].y, subreddits[position].z = positions[i][0], positions[i][1], positions[i][2]
		}
	}

	statement, err := tx.PrepareContext(ctx, pq.CopyIn("catalog_force_positions", "subreddit_id", "x", "y", "z", "provenance"))
	if err != nil {
		return result, fmt.Errorf("prepare force position copy: %w", err)
	}
	provenance := "force-layout"
	if !cold {
		provenance = "force-settled"
	}
	for _, entry := range subreddits {
		switch {
		case entry.body >= 0 && !nodes[entry.body].Pinned:
			if _, err := statement.ExecContext(ctx, entry.id, entry.x, entry.y, entry.z, provenance); err != nil {
				statement.Close()
				return result, fmt.Errorf("copy force position: %w", err)
			}
		case entry.body < 0 && !entry.seeded:
			x, y, z := ShellPosition("subreddit_"+strconv.Itoa(int(entry.id)), options.WorldRadius*isolatedShellInner, options.WorldRadius*isolatedShellOuter)
			if _, err := statement.ExecContext(ctx, entry.id, x, y, z, "deterministic-shell"); err != nil {
				statement.Close()
				return result, fmt.Errorf("copy shell position: %w", err)
			}
			result.Isolated++
		}
	}
	if _, err := statement.ExecContext(ctx); err != nil {
		statement.Close()
		return result, fmt.Errorf("flush force positions: %w", err)
	}
	if err := statement.Close(); err != nil {
		return result, fmt.Errorf("close force position copy: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `ANALYZE catalog_force_positions`); err != nil {
		return result, fmt.Errorf("analyze force positions: %w", err)
	}
	return result, nil
}

func mustSubredditID(entityID string) int {
	value, err := strconv.Atoi(entityID[len("subreddit_"):])
	if err != nil {
		panic(fmt.Sprintf("layout body %q is not a subreddit id", entityID))
	}
	return value
}
