package handlers

import (
	"context"

	"github.com/subculture-collective/clustr/backend/internal/logger"
)

// LogPprofAccess logs profiling endpoint access attempts for security monitoring.
// This helps track who is accessing sensitive runtime profiling data.
func LogPprofAccess(ctx context.Context, path, remoteAddr string) {
	logger.InfoContext(ctx, "Profiling endpoint accessed",
		"endpoint", path,
		"remote_addr", remoteAddr,
		"type", "security_audit")
}
