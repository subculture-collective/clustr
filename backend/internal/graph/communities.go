package graph

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"
	"sort"

	"github.com/lib/pq"
	"github.com/onnwee/reddit-cluster-map/backend/internal/db"
)

// Community represents a detected community with its members
type Community struct {
	ID      int
	Members []string
	Label   string
}

// CommunityResult holds the result of community detection
type CommunityResult struct {
	Communities     []Community
	NodeToCommunity map[string]int
	Modularity      float64
}

// HierarchyLevel represents one level in the community hierarchy
type HierarchyLevel struct {
	Level              int
	NodeToCommunity    map[string]int
	CommunityToParent  map[int]int
	CommunityCentroids map[int][3]float64 // community_id -> [x, y, z]
	Modularity         float64
}

// listGraphLinksAmongCapped keeps the hierarchy worker's Go adjacency map
// bounded even when the workspace retains a far larger detailed graph.  The
// ordering makes the sampled induced graph repeatable between runs.
func listGraphLinksAmongCapped(ctx context.Context, queries *db.Queries, nodeIDs []string, limit int) ([]db.ListGraphLinksAmongRow, error) {
	if limit < 1 {
		return nil, fmt.Errorf("community link cap must be positive")
	}
	rows, err := queries.DB().QueryContext(ctx, `SELECT source,target FROM graph_links
WHERE source=ANY($1::text[]) AND target=ANY($1::text[])
ORDER BY source,target LIMIT $2`, pq.Array(nodeIDs), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	links := make([]db.ListGraphLinksAmongRow, 0)
	for rows.Next() {
		var link db.ListGraphLinksAmongRow
		if err := rows.Scan(&link.Source, &link.Target); err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

// detectCommunities performs Louvain community detection on the graph
// Returns the community detection result along with the fetched nodes and links
func (s *Service) detectCommunities(ctx context.Context, queries *db.Queries) (*CommunityResult, []db.ListGraphNodesByWeightRow, []db.ListGraphLinksAmongRow, error) {
	log.Printf("🔍 Starting community detection (Louvain algorithm)")

	// Fetch all nodes and links
	nodes, err := queries.ListGraphNodesByWeight(ctx, 50000) // Cap at 50k nodes for performance
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fetch nodes: %w", err)
	}
	if len(nodes) == 0 {
		log.Printf("ℹ️ No nodes found for community detection")
		return &CommunityResult{Communities: []Community{}, NodeToCommunity: map[string]int{}, Modularity: 0}, nodes, nil, nil
	}

	nodeIDs := make([]string, len(nodes))
	for i, n := range nodes {
		nodeIDs[i] = n.ID
	}

	links, err := queries.ListGraphLinksAmong(ctx, nodeIDs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fetch links: %w", err)
	}

	result, err := s.detectCommunitiesFromData(nodes, links)
	return result, nodes, links, err
}

// detectCommunitiesFromData performs Louvain community detection on provided nodes and links
func (s *Service) detectCommunitiesFromData(nodes []db.ListGraphNodesByWeightRow, links []db.ListGraphLinksAmongRow) (*CommunityResult, error) {
	if len(nodes) == 0 {
		return &CommunityResult{Communities: []Community{}, NodeToCommunity: map[string]int{}, Modularity: 0}, nil
	}

	nodeIDs := make([]string, len(nodes))
	for i, n := range nodes {
		nodeIDs[i] = n.ID
	}

	log.Printf("📊 Building graph structure: %d nodes, %d links", len(nodeIDs), len(links))

	// Build adjacency map and degree map
	adjacency := make(map[string]map[string]int)
	degrees := make(map[string]int)
	for _, id := range nodeIDs {
		adjacency[id] = make(map[string]int)
		degrees[id] = 0
	}

	totalWeight := 0
	for _, link := range links {
		src := link.Source
		tgt := link.Target
		if _, ok := adjacency[src]; !ok {
			continue
		}
		if _, ok := adjacency[tgt]; !ok {
			continue
		}

		weight := 1
		adjacency[src][tgt] = adjacency[src][tgt] + weight
		adjacency[tgt][src] = adjacency[tgt][src] + weight
		degrees[src] += weight
		degrees[tgt] += weight
		totalWeight += weight
	}

	// Initialize each node to its own community
	nodeToCommunity := make(map[string]int)
	for i, id := range nodeIDs {
		nodeToCommunity[id] = i
	}

	// Louvain algorithm: iteratively move nodes to neighboring communities
	improved := true
	iteration := 0
	maxIterations := 50

	for improved && iteration < maxIterations {
		improved = false
		iteration++

		// Stable traversal and tie-breaking make identical projections repeatable.
		shuffled := make([]string, len(nodeIDs))
		copy(shuffled, nodeIDs)
		sort.Strings(shuffled)

		for _, nodeID := range shuffled {
			currentCommunity := nodeToCommunity[nodeID]
			neighbors := adjacency[nodeID]

			// Find neighboring communities
			neighborCommunities := make(map[int]bool)
			for neighbor := range neighbors {
				nComm := nodeToCommunity[neighbor]
				neighborCommunities[nComm] = true
			}

			bestCommunity := currentCommunity
			bestGain := 0.0

			// Try moving to each neighboring community
			orderedCommunities := make([]int, 0, len(neighborCommunities))
			for targetCommunity := range neighborCommunities {
				orderedCommunities = append(orderedCommunities, targetCommunity)
			}
			sort.Ints(orderedCommunities)
			for _, targetCommunity := range orderedCommunities {
				if targetCommunity == currentCommunity {
					continue
				}

				gain := modularityGain(nodeID, currentCommunity, targetCommunity, nodeToCommunity, adjacency, degrees, totalWeight)
				if gain > bestGain {
					bestGain = gain
					bestCommunity = targetCommunity
				}
			}

			// Move to best community if improvement found
			if bestCommunity != currentCommunity {
				nodeToCommunity[nodeID] = bestCommunity
				improved = true
			}
		}

		log.Printf("⏱ Iteration %d: improved=%v", iteration, improved)
	}

	// Renumber communities to be sequential
	uniqueCommunities := make(map[int]bool)
	for _, comm := range nodeToCommunity {
		uniqueCommunities[comm] = true
	}

	communityIDs := make([]int, 0, len(uniqueCommunities))
	for comm := range uniqueCommunities {
		communityIDs = append(communityIDs, comm)
	}
	sort.Ints(communityIDs)

	communityMap := make(map[int]int)
	for i, oldID := range communityIDs {
		communityMap[oldID] = i
	}

	finalNodeToCommunity := make(map[string]int)
	for node, comm := range nodeToCommunity {
		finalNodeToCommunity[node] = communityMap[comm]
	}

	// Build community information
	communitySizes := make(map[int][]string)
	for node, comm := range finalNodeToCommunity {
		communitySizes[comm] = append(communitySizes[comm], node)
	}

	// Calculate degree for each node
	nodeDegrees := make(map[string]int)
	for _, link := range links {
		nodeDegrees[link.Source]++
		nodeDegrees[link.Target]++
	}

	// Create communities with labels
	communities := make([]Community, 0, len(communitySizes))
	for id, members := range communitySizes {
		// Find top node to use as label
		topNode := ""
		maxDegree := -1
		for _, nodeID := range members {
			if deg := nodeDegrees[nodeID]; deg > maxDegree {
				maxDegree = deg
				topNode = nodeID
			}
		}

		label := topNode
		if label == "" {
			label = fmt.Sprintf("Community %d", id)
		} else {
			// Try to find the node name
			for _, n := range nodes {
				if n.ID == topNode {
					label = n.Name
					break
				}
			}
		}

		communities = append(communities, Community{
			ID:      id,
			Members: members,
			Label:   label,
		})
	}

	// Sort by size descending
	sort.Slice(communities, func(i, j int) bool {
		return len(communities[i].Members) > len(communities[j].Members)
	})

	// Calculate final modularity
	modularity := calculateModularity(finalNodeToCommunity, adjacency, degrees, totalWeight)

	log.Printf("✅ Community detection complete: %d communities, modularity=%.3f", len(communities), modularity)

	return &CommunityResult{
		Communities:     communities,
		NodeToCommunity: finalNodeToCommunity,
		Modularity:      modularity,
	}, nil
}

// modularityGain calculates the gain in modularity from moving a node between communities
func modularityGain(nodeID string, fromCommunity, toCommunity int, nodeToCommunity map[string]int, adjacency map[string]map[string]int, degrees map[string]int, totalWeight int) float64 {
	if totalWeight == 0 {
		return 0
	}

	neighbors := adjacency[nodeID]
	nodeDegree := degrees[nodeID]

	// Sum of weights to nodes in target community
	weightTo := 0
	// Sum of weights to nodes in current community
	weightFrom := 0

	for neighbor, weight := range neighbors {
		nComm := nodeToCommunity[neighbor]
		if nComm == toCommunity {
			weightTo += weight
		} else if nComm == fromCommunity {
			weightFrom += weight
		}
	}

	m2 := float64(2 * totalWeight)
	gain := (float64(weightTo-weightFrom) / m2) -
		(float64(nodeDegree) *
			(float64(sumDegrees(toCommunity, nodeToCommunity, degrees)) -
				float64(sumDegrees(fromCommunity, nodeToCommunity, degrees))) /
			(m2 * m2))

	return gain
}

// sumDegrees sums the degrees of all nodes in a community
func sumDegrees(community int, nodeToCommunity map[string]int, degrees map[string]int) int {
	sum := 0
	for node, comm := range nodeToCommunity {
		if comm == community {
			sum += degrees[node]
		}
	}
	return sum
}

// calculateModularity calculates the modularity of the current community structure
func calculateModularity(nodeToCommunity map[string]int, adjacency map[string]map[string]int, degrees map[string]int, totalWeight int) float64 {
	if totalWeight == 0 {
		return 0
	}

	modularity := 0.0
	m2 := float64(2 * totalWeight)

	for node1, neighbors := range adjacency {
		comm1 := nodeToCommunity[node1]
		deg1 := degrees[node1]

		for node2, weight := range neighbors {
			comm2 := nodeToCommunity[node2]
			if comm1 == comm2 {
				deg2 := degrees[node2]
				modularity += float64(weight) - (float64(deg1)*float64(deg2))/m2
			}
		}
	}

	return modularity / m2
}

// storeCommunities stores the detected communities in the database
// Accepts nodes and links fetched during detection to avoid redundant database queries
// Returns the mapping from node ID to database community ID for use in bundle computation
func (s *Service) storeCommunities(ctx context.Context, queries *db.Queries, result *CommunityResult, nodes []db.ListGraphNodesByWeightRow, links []db.ListGraphLinksAmongRow) (map[string]int32, error) {
	log.Printf("💾 Storing community detection results")

	// Clear existing communities
	if err := queries.ClearCommunityTables(ctx); err != nil {
		return nil, fmt.Errorf("clear community tables: %w", err)
	}

	// Insert communities and build mapping from member IDs to database community IDs
	nodeToDB := make(map[string]int32)
	for _, comm := range result.Communities {
		dbComm, err := queries.CreateCommunity(ctx, db.CreateCommunityParams{
			Label:      comm.Label,
			Size:       int32(len(comm.Members)),
			Modularity: sql.NullFloat64{Float64: result.Modularity, Valid: true},
		})
		if err != nil {
			return nil, fmt.Errorf("create community %d: %w", comm.ID, err)
		}

		// Insert members and map them to the database community ID
		for _, memberID := range comm.Members {
			if err := queries.CreateCommunityMember(ctx, db.CreateCommunityMemberParams{
				CommunityID: dbComm.ID,
				NodeID:      memberID,
			}); err != nil {
				return nil, fmt.Errorf("add member %s to community %d: %w", memberID, dbComm.ID, err)
			}
			nodeToDB[memberID] = dbComm.ID
		}
	}

	// Calculate and store inter-community links using the passed-in links
	linkWeights := make(map[[2]int32]int)

	// Count inter-community links
	for _, link := range links {
		commA, okA := nodeToDB[link.Source]
		commB, okB := nodeToDB[link.Target]
		if !okA || !okB || commA == commB {
			continue
		}

		// Create ordered pair
		var key [2]int32
		if commA < commB {
			key = [2]int32{commA, commB}
		} else {
			key = [2]int32{commB, commA}
		}
		linkWeights[key]++
	}

	// Store inter-community links
	for key, weight := range linkWeights {
		if err := queries.CreateCommunityLink(ctx, db.CreateCommunityLinkParams{
			SourceCommunityID: key[0],
			TargetCommunityID: key[1],
			Weight:            int32(weight),
		}); err != nil {
			return nil, fmt.Errorf("create community link %d->%d: %w", key[0], key[1], err)
		}
	}

	log.Printf("✅ Stored %d communities with inter-community links", len(result.Communities))
	return nodeToDB, nil
}

func communitiesFromHierarchy(hierarchy []HierarchyLevel) *CommunityResult {
	if len(hierarchy) == 0 {
		return &CommunityResult{NodeToCommunity: map[string]int{}}
	}
	level := hierarchy[0]
	if len(hierarchy) > 1 {
		level = hierarchy[1]
	}
	byID := make(map[int][]string)
	for nodeID, communityID := range level.NodeToCommunity {
		byID[communityID] = append(byID[communityID], nodeID)
	}
	ids := make([]int, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	communities := make([]Community, 0, len(ids))
	for _, id := range ids {
		sort.Strings(byID[id])
		communities = append(communities, Community{ID: id, Members: byID[id], Label: fmt.Sprintf("Community %d", id+1)})
	}
	return &CommunityResult{Communities: communities, NodeToCommunity: level.NodeToCommunity, Modularity: level.Modularity}
}

// computeAndStoreEdgeBundles computes edge bundle metadata for inter-community connections
// Bundles aggregate links between communities with control points for curved rendering
func (s *Service) computeAndStoreEdgeBundles(ctx context.Context, queries *db.Queries, nodeToCommunity map[string]int32, nodes []db.ListGraphNodesByWeightRow, links []db.ListGraphLinksAmongRow) error {
	log.Printf("🔄 Computing edge bundle metadata")

	// Calculate community centroids from node positions
	communityCentroids := make(map[int32][3]float64)
	communityNodeCounts := make(map[int32]int)

	nodePositions := make(map[string][3]float64)
	for _, n := range nodes {
		if n.PosX.Valid && n.PosY.Valid && n.PosZ.Valid {
			nodePositions[n.ID] = [3]float64{n.PosX.Float64, n.PosY.Float64, n.PosZ.Float64}
		}
	}

	// Accumulate positions per community
	for nodeID, commID := range nodeToCommunity {
		if pos, hasPos := nodePositions[nodeID]; hasPos {
			centroid := communityCentroids[commID]
			centroid[0] += pos[0]
			centroid[1] += pos[1]
			centroid[2] += pos[2]
			communityCentroids[commID] = centroid
			communityNodeCounts[commID]++
		}
	}

	// Average to get centroids
	for commID, count := range communityNodeCounts {
		if count > 0 {
			centroid := communityCentroids[commID]
			communityCentroids[commID] = [3]float64{
				centroid[0] / float64(count),
				centroid[1] / float64(count),
				centroid[2] / float64(count),
			}
		}
	}

	// Aggregate inter-community links
	type bundleKey struct{ src, tgt int32 }
	bundleWeights := make(map[bundleKey]int)

	for _, link := range links {
		srcComm, srcOK := nodeToCommunity[link.Source]
		tgtComm, tgtOK := nodeToCommunity[link.Target]

		if !srcOK || !tgtOK || srcComm == tgtComm {
			continue // Skip intra-community links
		}

		// Create ordered pair
		key := bundleKey{src: srcComm, tgt: tgtComm}
		if srcComm > tgtComm {
			key = bundleKey{src: tgtComm, tgt: srcComm}
		}

		bundleWeights[key]++
	}

	// Clear existing bundles
	if err := queries.ClearEdgeBundles(ctx); err != nil {
		return fmt.Errorf("clear edge bundles: %w", err)
	}

	// Store bundles with control points
	bundleCount := 0
	for key, weight := range bundleWeights {
		// Get community centroids
		srcCentroid, srcHasCentroid := communityCentroids[key.src]
		tgtCentroid, tgtHasCentroid := communityCentroids[key.tgt]

		var controlX, controlY, controlZ sql.NullFloat64

		if srcHasCentroid && tgtHasCentroid {
			// Calculate midpoint
			midX := (srcCentroid[0] + tgtCentroid[0]) / 2.0
			midY := (srcCentroid[1] + tgtCentroid[1]) / 2.0
			midZ := (srcCentroid[2] + tgtCentroid[2]) / 2.0

			// Calculate perpendicular offset for visual appeal
			// Use a vector perpendicular to the line between communities
			dx := tgtCentroid[0] - srcCentroid[0]
			dy := tgtCentroid[1] - srcCentroid[1]
			dz := tgtCentroid[2] - srcCentroid[2]

			// Get perpendicular vector (rotate 90 degrees in XY plane)
			perpX := -dy
			perpY := dx
			perpZ := 0.0

			// Normalize and scale
			perpLen := math.Sqrt(perpX*perpX + perpY*perpY + perpZ*perpZ)
			if perpLen > 0 {
				scale := math.Sqrt(dx*dx+dy*dy+dz*dz) * 0.2 // 20% of distance as offset
				perpX = (perpX / perpLen) * scale
				perpY = (perpY / perpLen) * scale
				perpZ = (perpZ / perpLen) * scale
			}

			// Apply offset to midpoint
			controlX = sql.NullFloat64{Float64: midX + perpX, Valid: true}
			controlY = sql.NullFloat64{Float64: midY + perpY, Valid: true}
			controlZ = sql.NullFloat64{Float64: midZ + perpZ, Valid: true}
		}

		// Store the bundle without avg_strength (set to NULL until we have real strength data)
		if err := queries.CreateEdgeBundle(ctx, db.CreateEdgeBundleParams{
			SourceCommunityID: key.src,
			TargetCommunityID: key.tgt,
			Weight:            int32(weight),
			AvgStrength:       sql.NullFloat64{Valid: false}, // NULL - no real strength data yet
			ControlX:          controlX,
			ControlY:          controlY,
			ControlZ:          controlZ,
		}); err != nil {
			log.Printf("⚠️ failed to create bundle %d->%d: %v", key.src, key.tgt, err)
		} else {
			bundleCount++
		}
	}

	log.Printf("✅ Stored %d edge bundles", bundleCount)
	return nil
}

// detectHierarchicalCommunities performs multi-level Louvain community detection
// Returns a hierarchy of levels where level 0 is the original nodes
func (s *Service) detectHierarchicalCommunities(ctx context.Context, queries *db.Queries, nodes []db.ListGraphNodesByWeightRow, links []db.ListGraphLinksAmongRow) ([]HierarchyLevel, error) {
	log.Printf("🔍 Starting hierarchical community detection (multi-level Louvain)")

	if len(nodes) == 0 {
		log.Printf("ℹ️ No nodes found for hierarchical community detection")
		return []HierarchyLevel{}, nil
	}

	nodeIDs := make([]string, len(nodes))
	for i, n := range nodes {
		nodeIDs[i] = n.ID
	}

	log.Printf("📊 Building graph structure: %d nodes, %d links", len(nodeIDs), len(links))

	// Build initial adjacency map and degree map
	adjacency := make(map[string]map[string]int)
	degrees := make(map[string]int)
	for _, id := range nodeIDs {
		adjacency[id] = make(map[string]int)
		degrees[id] = 0
	}

	totalWeight := 0
	for _, link := range links {
		src := link.Source
		tgt := link.Target
		if _, ok := adjacency[src]; !ok {
			continue
		}
		if _, ok := adjacency[tgt]; !ok {
			continue
		}

		weight := 1
		adjacency[src][tgt] = adjacency[src][tgt] + weight
		adjacency[tgt][src] = adjacency[tgt][src] + weight
		degrees[src] += weight
		degrees[tgt] += weight
		totalWeight += weight
	}

	// Level 0: each node is its own community
	level0 := HierarchyLevel{
		Level:              0,
		NodeToCommunity:    make(map[string]int),
		CommunityToParent:  make(map[int]int),
		CommunityCentroids: make(map[int][3]float64),
		Modularity:         0,
	}
	for i, id := range nodeIDs {
		level0.NodeToCommunity[id] = i
	}
	// Calculate initial centroids from node positions
	level0.CommunityCentroids = s.calculateCentroidsForLevel(ctx, queries, level0.NodeToCommunity, nodes)

	hierarchy := []HierarchyLevel{level0}

	// Iteratively apply Louvain to build hierarchy levels
	currentNodeIDs := nodeIDs
	currentAdjacency := adjacency
	currentDegrees := degrees
	currentTotalWeight := totalWeight
	maxLevels := 4 // Target 3-4 levels (0 is original, 1-3 are hierarchy)

	for level := 1; level < maxLevels; level++ {
		log.Printf("🔄 Computing hierarchy level %d", level)

		// Run single-pass Louvain on current level
		metaNodeToCommunity := runSinglePassLouvain(currentNodeIDs, currentAdjacency, currentDegrees, currentTotalWeight)

		// Check if we got any clustering (more than 1 community and less than total nodes)
		uniqueCommunities := make(map[int]bool)
		for _, comm := range metaNodeToCommunity {
			uniqueCommunities[comm] = true
		}

		if len(uniqueCommunities) <= 1 || len(uniqueCommunities) >= len(currentNodeIDs) {
			log.Printf("⚠️ Level %d: clustering not effective (%d communities for %d nodes), stopping hierarchy", level, len(uniqueCommunities), len(currentNodeIDs))
			if level > 1 && len(hierarchy) > 1 {
				rootMembers := make(map[string]int, len(nodeIDs))
				for _, nodeID := range nodeIDs {
					rootMembers[nodeID] = 0
				}
				hierarchy = append(hierarchy, HierarchyLevel{
					Level:              level,
					NodeToCommunity:    rootMembers,
					CommunityToParent:  map[int]int{},
					CommunityCentroids: s.calculateCentroidsForLevel(ctx, queries, rootMembers, nodes),
					Modularity:         hierarchy[len(hierarchy)-1].Modularity,
				})
				log.Printf("✅ Level %d: added deterministic world root", level)
			}
			break
		}

		// Renumber communities sequentially
		communityIDs := make([]int, 0, len(uniqueCommunities))
		for comm := range uniqueCommunities {
			communityIDs = append(communityIDs, comm)
		}
		sort.Ints(communityIDs)

		communityMap := make(map[int]int)
		for i, oldID := range communityIDs {
			communityMap[oldID] = i
		}

		// For level 1, currentNodeIDs are the original node IDs
		// For level 2+, currentNodeIDs are meta-node IDs from previous level
		// We need to map original nodes to their new communities

		finalNodeToCommunity := make(map[string]int)

		if level == 1 {
			// First level: currentNodeIDs are original nodes
			for node, comm := range metaNodeToCommunity {
				finalNodeToCommunity[node] = communityMap[comm]
			}
		} else {
			// Higher levels: map through meta-nodes back to original nodes
			// Each meta-node represents a community from the previous level
			prevLevel := hierarchy[len(hierarchy)-1]

			// Build reverse mapping: which original nodes are in which prev-level community
			prevCommToNodes := make(map[int][]string)
			for origNode, prevComm := range prevLevel.NodeToCommunity {
				prevCommToNodes[prevComm] = append(prevCommToNodes[prevComm], origNode)
			}

			// Now map: meta-node -> new community -> original nodes
			for metaNode, oldComm := range metaNodeToCommunity {
				newComm := communityMap[oldComm]
				// Parse meta-node ID to get the previous community it represents
				var prevCommID int
				fmt.Sscanf(metaNode, "meta_%d", &prevCommID)

				// All original nodes in that previous community now belong to newComm
				for _, origNode := range prevCommToNodes[prevCommID] {
					finalNodeToCommunity[origNode] = newComm
				}
			}
		}

		// Calculate modularity on the meta-graph level
		modularity := calculateModularity(metaNodeToCommunity, currentAdjacency, currentDegrees, currentTotalWeight)

		// Map to parent communities from previous level
		// A new community can merge multiple parent communities, so we mark mixed parents as -1
		parentLevel := hierarchy[len(hierarchy)-1]
		communityToParent := make(map[int]int)
		for origNode, newComm := range finalNodeToCommunity {
			oldComm := parentLevel.NodeToCommunity[origNode]
			if oldParent, exists := communityToParent[newComm]; exists {
				if oldParent != oldComm && oldParent != -1 {
					// This community has nodes from multiple parent communities; mark as mixed
					communityToParent[newComm] = -1
				}
			} else {
				// First time seeing this community, store the parent; may be marked mixed later
				communityToParent[newComm] = oldComm
			}
		}

		// Calculate centroids for this level
		centroids := s.calculateCentroidsForLevel(ctx, queries, finalNodeToCommunity, nodes)

		newLevel := HierarchyLevel{
			Level:              level,
			NodeToCommunity:    finalNodeToCommunity,
			CommunityToParent:  communityToParent,
			CommunityCentroids: centroids,
			Modularity:         modularity,
		}
		hierarchy = append(hierarchy, newLevel)

		log.Printf("✅ Level %d: %d communities, modularity=%.3f", level, len(uniqueCommunities), modularity)

		// Build meta-graph for next level: communities become nodes
		metaNodeIDs := make([]string, 0, len(uniqueCommunities))
		metaAdjacency := make(map[string]map[string]int)
		metaDegrees := make(map[string]int)
		metaTotalWeight := 0

		// Create meta-nodes (one per community)
		for _, newCommID := range communityMap {
			metaNodeID := fmt.Sprintf("meta_%d", newCommID)
			metaNodeIDs = append(metaNodeIDs, metaNodeID)
			metaAdjacency[metaNodeID] = make(map[string]int)
			metaDegrees[metaNodeID] = 0
		}

		// Aggregate links between communities (including self-loops for intra-community edges)
		for node1, neighbors := range currentAdjacency {
			comm1 := metaNodeToCommunity[node1]
			metaNode1 := fmt.Sprintf("meta_%d", communityMap[comm1])

			for node2, weight := range neighbors {
				comm2 := metaNodeToCommunity[node2]
				metaNode2 := fmt.Sprintf("meta_%d", communityMap[comm2])

				// Add edge weight (self-loops for intra-community edges)
				metaAdjacency[metaNode1][metaNode2] += weight
				metaDegrees[metaNode1] += weight
				metaTotalWeight += weight
			}
		}

		// If meta-graph is too small, stop
		if len(metaNodeIDs) < 3 {
			log.Printf("ℹ️ Meta-graph too small (%d communities), stopping hierarchy", len(metaNodeIDs))
			break
		}

		// Continue with meta-graph
		currentNodeIDs = metaNodeIDs
		currentAdjacency = metaAdjacency
		currentDegrees = metaDegrees
		currentTotalWeight = metaTotalWeight
	}

	log.Printf("🎉 Hierarchical community detection complete: %d levels", len(hierarchy))
	return hierarchy, nil
}

// runSinglePassLouvain performs one pass of Louvain algorithm on the given graph
func runSinglePassLouvain(nodeIDs []string, adjacency map[string]map[string]int, degrees map[string]int, totalWeight int) map[string]int {
	// Initialize each node to its own community
	nodeToCommunity := make(map[string]int)
	for i, id := range nodeIDs {
		nodeToCommunity[id] = i
	}

	// Louvain algorithm: iteratively move nodes to neighboring communities
	improved := true
	iteration := 0
	maxIterations := 50

	for improved && iteration < maxIterations {
		improved = false
		iteration++

		// Stable traversal and tie-breaking make identical projections repeatable.
		shuffled := make([]string, len(nodeIDs))
		copy(shuffled, nodeIDs)
		sort.Strings(shuffled)

		for _, nodeID := range shuffled {
			currentCommunity := nodeToCommunity[nodeID]
			neighbors := adjacency[nodeID]

			// Find neighboring communities
			neighborCommunities := make(map[int]bool)
			for neighbor := range neighbors {
				nComm := nodeToCommunity[neighbor]
				neighborCommunities[nComm] = true
			}

			bestCommunity := currentCommunity
			bestGain := 0.0

			// Try moving to each neighboring community
			orderedCommunities := make([]int, 0, len(neighborCommunities))
			for targetCommunity := range neighborCommunities {
				orderedCommunities = append(orderedCommunities, targetCommunity)
			}
			sort.Ints(orderedCommunities)
			for _, targetCommunity := range orderedCommunities {
				if targetCommunity == currentCommunity {
					continue
				}

				gain := modularityGain(nodeID, currentCommunity, targetCommunity, nodeToCommunity, adjacency, degrees, totalWeight)
				if gain > bestGain {
					bestGain = gain
					bestCommunity = targetCommunity
				}
			}

			// Move to best community if improvement found
			if bestCommunity != currentCommunity {
				nodeToCommunity[nodeID] = bestCommunity
				improved = true
			}
		}
	}

	return nodeToCommunity
}

// calculateCentroidsForLevel computes the centroid positions for each community in a level
func (s *Service) calculateCentroidsForLevel(ctx context.Context, queries *db.Queries, nodeToCommunity map[string]int, nodes []db.ListGraphNodesByWeightRow) map[int][3]float64 {
	centroids := make(map[int][3]float64)
	communityCounts := make(map[int]int)
	communitySum := make(map[int][3]float64)

	// Build node position lookup
	nodePositions := make(map[string][3]float64)
	nodeHasPosition := make(map[string]bool)
	for _, n := range nodes {
		if n.PosX.Valid && n.PosY.Valid && n.PosZ.Valid {
			nodePositions[n.ID] = [3]float64{n.PosX.Float64, n.PosY.Float64, n.PosZ.Float64}
			nodeHasPosition[n.ID] = true
		}
	}

	// Accumulate positions per community
	for nodeID, commID := range nodeToCommunity {
		if nodeHasPosition[nodeID] {
			pos := nodePositions[nodeID]
			sum := communitySum[commID]
			sum[0] += pos[0]
			sum[1] += pos[1]
			sum[2] += pos[2]
			communitySum[commID] = sum
			communityCounts[commID]++
		}
	}

	// Calculate centroids
	for commID, sum := range communitySum {
		count := communityCounts[commID]
		if count > 0 {
			centroids[commID] = [3]float64{
				sum[0] / float64(count),
				sum[1] / float64(count),
				sum[2] / float64(count),
			}
		}
	}

	return centroids
}

// storeHierarchy stores the hierarchical community structure in the database
func (s *Service) storeHierarchy(ctx context.Context, queries *db.Queries, hierarchy []HierarchyLevel) error {
	log.Printf("💾 Storing hierarchical community structure")

	// Clear existing hierarchy
	if err := queries.ClearCommunityHierarchy(ctx); err != nil {
		return fmt.Errorf("clear community hierarchy: %w", err)
	}

	// Store each level
	totalRows := 0
	var firstError error
	for _, level := range hierarchy {
		for nodeID, commID := range level.NodeToCommunity {
			var parentCommID sql.NullInt32
			if parent, ok := level.CommunityToParent[commID]; ok && parent != -1 {
				parentCommID = sql.NullInt32{Int32: int32(parent), Valid: true}
			}

			// Check if centroid exists for this community
			centroid, hasCentroid := level.CommunityCentroids[commID]
			var cx, cy, cz sql.NullFloat64
			if hasCentroid {
				cx = sql.NullFloat64{Float64: centroid[0], Valid: true}
				cy = sql.NullFloat64{Float64: centroid[1], Valid: true}
				cz = sql.NullFloat64{Float64: centroid[2], Valid: true}
			}

			if err := queries.InsertCommunityHierarchy(ctx, db.InsertCommunityHierarchyParams{
				NodeID:            nodeID,
				Level:             int32(level.Level),
				CommunityID:       int32(commID),
				ParentCommunityID: parentCommID,
				CentroidX:         cx,
				CentroidY:         cy,
				CentroidZ:         cz,
			}); err != nil {
				log.Printf("⚠️ failed to insert hierarchy row for node %s at level %d: %v", nodeID, level.Level, err)
				if firstError == nil {
					firstError = fmt.Errorf("failed to insert hierarchy row for node %s at level %d: %w", nodeID, level.Level, err)
				}
			} else {
				totalRows++
			}
		}
		log.Printf("✅ Stored level %d: %d rows", level.Level, len(level.NodeToCommunity))
	}

	log.Printf("✅ Stored hierarchical community structure: %d total rows across %d levels", totalRows, len(hierarchy))

	if firstError != nil {
		return firstError
	}
	return nil
}
