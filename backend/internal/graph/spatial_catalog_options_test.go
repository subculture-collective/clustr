package graph

import (
	"context"
	"testing"
	"time"
)

func TestDefaultSpatialCatalogOptionsReadsParallelWorkers(t *testing.T) {
	t.Setenv("SPATIAL_CATALOG_PARALLEL_WORKERS", "4")
	got := DefaultSpatialCatalogOptions()
	if got.ParallelWorkers != 4 {
		t.Fatalf("ParallelWorkers = %d, want 4", got.ParallelWorkers)
	}
}

func TestDefaultSpatialCatalogOptionsAllowsSafeZeroParallelism(t *testing.T) {
	t.Setenv("SPATIAL_CATALOG_PARALLEL_WORKERS", "0")
	if got := DefaultSpatialCatalogOptions().ParallelWorkers; got != 0 {
		t.Fatalf("ParallelWorkers = %d, want 0", got)
	}
}

func TestSpatialCatalogOptionsRejectNegativeParallelismBeforeDatabaseUse(t *testing.T) {
	options := SpatialCatalogOptions{Retention: 1, WorkMemMB: 16, ParallelWorkers: -1}
	if _, err := BuildSpatialCatalogWithOptions(context.Background(), nil, time.Now(), options); err == nil {
		t.Fatal("expected negative parallel workers to be rejected")
	}
}
