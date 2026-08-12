package graph

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/onnwee/reddit-cluster-map/backend/internal/config"
	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
)

// GraphNode represents a simplified node for diff comparison
type GraphNode struct {
	ID   string
	Name string
	Val  string
	Type string
	PosX sql.NullFloat64
	PosY sql.NullFloat64
	PosZ sql.NullFloat64
}

// GraphLink represents a simplified link for diff comparison
type GraphLink struct {
	Source string
	Target string
}

// GraphSnapshot represents the state of the graph at a point in time
type GraphSnapshot struct {
	Nodes map[string]GraphNode
	Links map[string]GraphLink // key is "source->target"
}

// VersionStore defines the operations needed for version tracking
// It embeds GraphStore which already includes all necessary version tracking methods
type VersionStore interface {
	GraphStore // Embed the existing GraphStore interface
}

// CaptureGraphSnapshot captures the current state of the graph for diff comparison
func CaptureGraphSnapshot(ctx context.Context, store VersionStore) (*GraphSnapshot, error) {
	// Fetch all current nodes and links (use a high limit to get everything)
	rows, err := store.GetPrecalculatedGraphDataCappedAll(ctx, db.GetPrecalculatedGraphDataCappedAllParams{
		Limit:   1000000, // max nodes
		Limit_2: 5000000, // max links
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch graph data: %w", err)
	}

	snapshot := &GraphSnapshot{
		Nodes: make(map[string]GraphNode),
		Links: make(map[string]GraphLink),
	}

	for _, row := range rows {
		if row.DataType == "node" {
			snapshot.Nodes[row.ID] = GraphNode{
				ID:   row.ID,
				Name: row.Name,
				Val:  row.Val,
				Type: row.Type.String,
				PosX: row.PosX,
				PosY: row.PosY,
				PosZ: row.PosZ,
			}
		} else if row.DataType == "link" {
			// Source and Target are interface{}, handle both string and []byte
			var source, target string
			switch v := row.Source.(type) {
			case string:
				source = v
			case []byte:
				source = string(v)
			default:
				log.Printf("⚠️ Unexpected type for row.Source: %T", row.Source)
				continue
			}
			switch v := row.Target.(type) {
			case string:
				target = v
			case []byte:
				target = string(v)
			default:
				log.Printf("⚠️ Unexpected type for row.Target: %T", row.Target)
				continue
			}
			if source == "" || target == "" {
				log.Printf("⚠️ Empty source or target in link: source=%q, target=%q", source, target)
				continue
			}
			key := fmt.Sprintf("%s->%s", source, target)
			snapshot.Links[key] = GraphLink{
				Source: source,
				Target: target,
			}
		}
	}

	log.Printf("📸 Captured graph snapshot: %d nodes, %d links", len(snapshot.Nodes), len(snapshot.Links))
	return snapshot, nil
}

// CalculateAndStoreDiffs compares two snapshots and stores the differences
func CalculateAndStoreDiffs(ctx context.Context, store VersionStore, versionID int64, oldSnapshot, newSnapshot *GraphSnapshot) error {
	if oldSnapshot == nil {
		// First version - all nodes and links are "add" operations
		log.Printf("📊 First version - storing all %d nodes and %d links as additions", len(newSnapshot.Nodes), len(newSnapshot.Links))

		// Store node additions
		for _, node := range newSnapshot.Nodes {
			if err := store.CreateGraphDiff(ctx, db.CreateGraphDiffParams{
				VersionID:  versionID,
				Action:     "add",
				EntityType: "node",
				EntityID:   node.ID,
				OldVal:     sql.NullString{},
				NewVal:     sql.NullString{String: node.Val, Valid: true},
				NewPosX:    node.PosX,
				NewPosY:    node.PosY,
				NewPosZ:    node.PosZ,
			}); err != nil {
				return fmt.Errorf("failed to store node diff: %w", err)
			}
		}

		// Store link additions
		for _, link := range newSnapshot.Links {
			linkID := fmt.Sprintf("%s->%s", link.Source, link.Target)
			if err := store.CreateGraphDiff(ctx, db.CreateGraphDiffParams{
				VersionID:  versionID,
				Action:     "add",
				EntityType: "link",
				EntityID:   linkID,
			}); err != nil {
				return fmt.Errorf("failed to store link diff: %w", err)
			}
		}

		return nil
	}

	// Calculate diffs
	var nodeAdds, nodeRemoves, nodeUpdates int
	var linkAdds, linkRemoves int

	// Find added and updated nodes
	for nodeID, newNode := range newSnapshot.Nodes {
		if oldNode, exists := oldSnapshot.Nodes[nodeID]; !exists {
			// Node added
			if err := store.CreateGraphDiff(ctx, db.CreateGraphDiffParams{
				VersionID:  versionID,
				Action:     "add",
				EntityType: "node",
				EntityID:   nodeID,
				NewVal:     sql.NullString{String: newNode.Val, Valid: true},
				NewPosX:    newNode.PosX,
				NewPosY:    newNode.PosY,
				NewPosZ:    newNode.PosZ,
			}); err != nil {
				return fmt.Errorf("failed to store node add diff: %w", err)
			}
			nodeAdds++
		} else {
			// Check if node was updated (value or position changed)
			valChanged := oldNode.Val != newNode.Val
			posChanged := !equalNullFloat(oldNode.PosX, newNode.PosX) ||
				!equalNullFloat(oldNode.PosY, newNode.PosY) ||
				!equalNullFloat(oldNode.PosZ, newNode.PosZ)

			if valChanged || posChanged {
				if err := store.CreateGraphDiff(ctx, db.CreateGraphDiffParams{
					VersionID:  versionID,
					Action:     "update",
					EntityType: "node",
					EntityID:   nodeID,
					OldVal:     sql.NullString{String: oldNode.Val, Valid: true},
					NewVal:     sql.NullString{String: newNode.Val, Valid: true},
					OldPosX:    oldNode.PosX,
					OldPosY:    oldNode.PosY,
					OldPosZ:    oldNode.PosZ,
					NewPosX:    newNode.PosX,
					NewPosY:    newNode.PosY,
					NewPosZ:    newNode.PosZ,
				}); err != nil {
					return fmt.Errorf("failed to store node update diff: %w", err)
				}
				nodeUpdates++
			}
		}
	}

	// Find removed nodes
	for nodeID, oldNode := range oldSnapshot.Nodes {
		if _, exists := newSnapshot.Nodes[nodeID]; !exists {
			if err := store.CreateGraphDiff(ctx, db.CreateGraphDiffParams{
				VersionID:  versionID,
				Action:     "remove",
				EntityType: "node",
				EntityID:   nodeID,
				OldVal:     sql.NullString{String: oldNode.Val, Valid: true},
				OldPosX:    oldNode.PosX,
				OldPosY:    oldNode.PosY,
				OldPosZ:    oldNode.PosZ,
			}); err != nil {
				return fmt.Errorf("failed to store node remove diff: %w", err)
			}
			nodeRemoves++
		}
	}

	// Find added links
	for linkKey := range newSnapshot.Links {
		if _, exists := oldSnapshot.Links[linkKey]; !exists {
			if err := store.CreateGraphDiff(ctx, db.CreateGraphDiffParams{
				VersionID:  versionID,
				Action:     "add",
				EntityType: "link",
				EntityID:   linkKey,
			}); err != nil {
				return fmt.Errorf("failed to store link add diff: %w", err)
			}
			linkAdds++
		}
	}

	// Find removed links
	for linkKey := range oldSnapshot.Links {
		if _, exists := newSnapshot.Links[linkKey]; !exists {
			if err := store.CreateGraphDiff(ctx, db.CreateGraphDiffParams{
				VersionID:  versionID,
				Action:     "remove",
				EntityType: "link",
				EntityID:   linkKey,
			}); err != nil {
				return fmt.Errorf("failed to store link remove diff: %w", err)
			}
			linkRemoves++
		}
	}

	log.Printf("📊 Diff calculated - Nodes: +%d -%d ~%d | Links: +%d -%d",
		nodeAdds, nodeRemoves, nodeUpdates, linkAdds, linkRemoves)

	return nil
}

// equalNullFloat compares two sql.NullFloat64 values
func equalNullFloat(a, b sql.NullFloat64) bool {
	if a.Valid != b.Valid {
		return false
	}
	if !a.Valid {
		return true // both are NULL
	}
	// Compare floats with small epsilon for floating point precision
	const epsilon = 0.0001
	diff := a.Float64 - b.Float64
	if diff < 0 {
		diff = -diff
	}
	return diff < epsilon
}

// CleanupOldVersions removes old graph versions beyond the retention limit
func CleanupOldVersions(ctx context.Context, store VersionStore) error {
	cfg := config.Load()
	retention := cfg.GetEnvInt("GRAPH_VERSION_RETENTION", 10)

	// Validate retention value (minimum 1 to prevent deleting all versions)
	if retention < 1 {
		log.Printf("⚠️ Invalid GRAPH_VERSION_RETENTION value %d, using minimum of 1", retention)
		retention = 1
	}

	count, err := store.CountGraphVersions(ctx)
	if err != nil {
		return fmt.Errorf("failed to count versions: %w", err)
	}

	if count <= int64(retention) {
		log.Printf("ℹ️ Version count (%d) within retention limit (%d), no cleanup needed", count, retention)
		return nil
	}

	log.Printf("🧹 Cleaning up old versions (keeping most recent %d of %d)", retention, count)

	if err := store.DeleteOldGraphVersions(ctx, int32(retention)); err != nil {
		return fmt.Errorf("failed to delete old versions: %w", err)
	}

	log.Printf("✅ Old versions cleaned up")
	return nil
}

// CreateVersionAndTrackChanges creates a new graph version and tracks changes from the previous state
func CreateVersionAndTrackChanges(ctx context.Context, store VersionStore, nodeCount, linkCount int32, isFullRebuild bool, durationMs int32) (int64, error) {
	// Create the new version record
	version, err := store.CreateGraphVersion(ctx, db.CreateGraphVersionParams{
		NodeCount:         nodeCount,
		LinkCount:         linkCount,
		Status:            "completed",
		PrecalcDurationMs: sql.NullInt32{Int32: durationMs, Valid: true},
		IsFullRebuild:     isFullRebuild,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to create graph version: %w", err)
	}

	log.Printf("📌 Created graph version %d (nodes: %d, links: %d)", version.ID, nodeCount, linkCount)

	// Update precalc state with current version
	if err := store.UpdatePrecalcStateVersion(ctx, sql.NullInt64{Int64: version.ID, Valid: true}); err != nil {
		log.Printf("⚠️ Failed to update precalc state version: %v", err)
		// Non-fatal - continue
	}

	return version.ID, nil
}

// CountGraphEntities counts the current nodes and links in the graph
func CountGraphEntities(ctx context.Context, store VersionStore) (nodeCount, linkCount int32, err error) {
	// Fetch a snapshot with high limits to count everything
	rows, err := store.GetPrecalculatedGraphDataCappedAll(ctx, db.GetPrecalculatedGraphDataCappedAllParams{
		Limit:   1000000, // max nodes
		Limit_2: 5000000, // max links
	})
	if err != nil {
		return 0, 0, fmt.Errorf("failed to fetch graph data: %w", err)
	}

	nodes := 0
	links := 0
	for _, row := range rows {
		if row.DataType == "node" {
			nodes++
		} else if row.DataType == "link" {
			links++
		}
	}

	return int32(nodes), int32(links), nil
}
