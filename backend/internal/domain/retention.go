package domain

import "time"

// AgeCutoff returns the time point that is beforeDays days before now.
// Used by PurgeByAge to compute the cutoff for deletion.
func AgeCutoff(beforeDays int) time.Time {
	return time.Now().Add(-time.Duration(beforeDays) * 24 * time.Hour)
}

// RetentionConfig holds per-project data retention settings.
type RetentionConfig struct {
	ProjectID     string    `json:"project_id"`
	RetentionDays int       `json:"retention_days"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// StorageStats summarises storage usage across all backends for a project.
// Byte estimates are approximations — StatsApprox is always true.
type StorageStats struct {
	ProjectID     string     `json:"project_id"`
	SpanRowCount  int64      `json:"span_row_count"`
	SpanBytesEst  int64      `json:"span_bytes_est"`  // estimated, not exact
	TopologyRows  int64      `json:"topology_rows"`
	S3ObjectCount int64      `json:"s3_object_count"`
	S3Bytes       int64      `json:"s3_bytes"`
	OldestSpanAt  *time.Time `json:"oldest_span_at"`
	NewestSpanAt  *time.Time `json:"newest_span_at"`
	StatsApprox   bool       `json:"stats_approximate"` // always true for byte estimates
	ComputedAt    time.Time  `json:"computed_at"`
}

// PurgeJob tracks the progress and result of an async data purge.
type PurgeJob struct {
	ID             string     `json:"id"`
	ProjectID      string     `json:"project_id"`
	RunID          string     `json:"run_id,omitempty"`
	CutoffDate     *time.Time `json:"cutoff_date,omitempty"`
	Status         string     `json:"status"`
	IncludeEvals   bool       `json:"include_evals"`
	SpansDeleted   int64      `json:"spans_deleted"`
	S3KeysDeleted  int64      `json:"s3_keys_deleted"`
	PGRowsDeleted  int64      `json:"pg_rows_deleted"`
	PartialFailure bool       `json:"partial_failure"`
	ErrorMsg       string     `json:"error_msg,omitempty"`
	StartedAt      time.Time  `json:"started_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
}
