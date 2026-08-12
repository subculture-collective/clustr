package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

// BatchUpsertGraphNodes performs a multi-row upsert for graph_nodes for the provided slice.
// It falls back to no-op if the slice is empty. Batch size limits the number of rows per statement.
func (q *Queries) BatchUpsertGraphNodes(ctx context.Context, nodes []BulkInsertGraphNodeParams, batchSize int) error {
	if len(nodes) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = 1000
	}
	// Build batches
	for start := 0; start < len(nodes); start += batchSize {
		end := start + batchSize
		if end > len(nodes) {
			end = len(nodes)
		}
		batch := nodes[start:end]
		var sb strings.Builder
		sb.WriteString("INSERT INTO graph_nodes (id,name,val,type) VALUES ")
		args := make([]any, 0, len(batch)*4)
		for i, n := range batch {
			if i > 0 {
				sb.WriteByte(',')
			}
			idx := i*4 + 1
			sb.WriteString(fmt.Sprintf("($%d,$%d,$%d,$%d)", idx, idx+1, idx+2, idx+3))
			args = append(args, n.ID, n.Name, n.Val, n.Type)
		}
		sb.WriteString(" ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name,val=EXCLUDED.val,type=EXCLUDED.type,updated_at=now()")
		if _, err := q.db.ExecContext(ctx, sb.String(), args...); err != nil {
			return err
		}
	}
	return nil
}

// BatchInsertGraphLinks inserts many graph_links rows with ON CONFLICT DO NOTHING semantics in batches.
// It de-duplicates (source,target) pairs client-side to reduce useless conflict checks.
func (q *Queries) BatchInsertGraphLinks(ctx context.Context, links []BulkInsertGraphLinkParams, batchSize int) error {
	if len(links) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = 2000
	}
	// Deduplicate
	uniq := make(map[string]BulkInsertGraphLinkParams, len(links))
	for _, l := range links {
		key := l.Source + "\x00" + l.Target
		uniq[key] = l
	}
	dedup := make([]BulkInsertGraphLinkParams, 0, len(uniq))
	for _, v := range uniq {
		dedup = append(dedup, v)
	}
	// Batch insert using a VALUES table joined against graph_nodes to satisfy FKs
	for start := 0; start < len(dedup); start += batchSize {
		end := start + batchSize
		if end > len(dedup) {
			end = len(dedup)
		}
		batch := dedup[start:end]
		var sb strings.Builder
		sb.WriteString("WITH vals(source, target) AS (VALUES ")
		args := make([]any, 0, len(batch)*2)
		for i, l := range batch {
			if i > 0 {
				sb.WriteByte(',')
			}
			idx := i*2 + 1
			sb.WriteString(fmt.Sprintf("($%d,$%d)", idx, idx+1))
			args = append(args, l.Source, l.Target)
		}
		sb.WriteString(") INSERT INTO graph_links (source, target) ")
		sb.WriteString("SELECT v.source, v.target FROM vals v ")
		sb.WriteString("JOIN graph_nodes s ON s.id = v.source ")
		sb.WriteString("JOIN graph_nodes t ON t.id = v.target ")
		sb.WriteString("ON CONFLICT (source, target) DO NOTHING")
		if _, err := q.db.ExecContext(ctx, sb.String(), args...); err != nil {
			return err
		}
	}
	return nil
}

// BatchUpdateGraphNodePositions updates positions for multiple nodes in chunks to avoid large array binds
// and reduce lock contention. It filters out nodes with negligible changes if epsilon > 0.
// Returns the number of nodes updated.
func (q *Queries) BatchUpdateGraphNodePositions(ctx context.Context, ids []string, x, y, z []float64, batchSize int, epsilon float64) (int, error) {
	if len(ids) == 0 || len(ids) != len(x) || len(ids) != len(y) || len(ids) != len(z) {
		return 0, fmt.Errorf("ids, x, y, z arrays must have the same non-zero length")
	}
	if batchSize <= 0 {
		batchSize = 5000
	}

	// Apply epsilon filtering if needed (only update if position changed significantly)
	filtered := make([]int, 0, len(ids))
	if epsilon > 0 {
		// Query existing positions for comparison
		query := "SELECT id, pos_x, pos_y, pos_z FROM graph_nodes WHERE id = ANY($1)"
		rows, err := q.db.QueryContext(ctx, query, pq.Array(ids))
		if err != nil {
			return 0, fmt.Errorf("failed to query existing positions: %w", err)
		}
		defer rows.Close()

		existing := make(map[string][3]float64)
		for rows.Next() {
			var id string
			var px, py, pz *float64
			if err := rows.Scan(&id, &px, &py, &pz); err != nil {
				return 0, fmt.Errorf("failed to scan position: %w", err)
			}
			if px != nil && py != nil && pz != nil {
				existing[id] = [3]float64{*px, *py, *pz}
			}
		}

		// Filter based on epsilon threshold
		for i := range ids {
			if oldPos, ok := existing[ids[i]]; ok {
				dx := x[i] - oldPos[0]
				dy := y[i] - oldPos[1]
				dz := z[i] - oldPos[2]
				distSq := dx*dx + dy*dy + dz*dz
				if distSq < epsilon*epsilon {
					continue // Skip if change is below threshold
				}
			}
			filtered = append(filtered, i)
		}
	} else {
		// No filtering, update all
		for i := range ids {
			filtered = append(filtered, i)
		}
	}

	if len(filtered) == 0 {
		return 0, nil
	}

	totalUpdated := 0
	// Process in batches
	for start := 0; start < len(filtered); start += batchSize {
		end := start + batchSize
		if end > len(filtered) {
			end = len(filtered)
		}

		// Build batch arrays
		batchIDs := make([]string, end-start)
		batchX := make([]float64, end-start)
		batchY := make([]float64, end-start)
		batchZ := make([]float64, end-start)

		for i := start; i < end; i++ {
			idx := filtered[i]
			batchIDs[i-start] = ids[idx]
			batchX[i-start] = x[idx]
			batchY[i-start] = y[idx]
			batchZ[i-start] = z[idx]
		}

		// Execute batch update
		if err := q.UpdateGraphNodePositions(ctx, UpdateGraphNodePositionsParams{
			Column1: batchIDs,
			Column2: batchX,
			Column3: batchY,
			Column4: batchZ,
		}); err != nil {
			return totalUpdated, fmt.Errorf("failed to update batch %d-%d: %w", start, end, err)
		}
		totalUpdated += len(batchIDs)
	}

	return totalUpdated, nil
}
