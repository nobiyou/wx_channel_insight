package ingest

// MetricMask records unavailable source fields without changing the legacy
// scalar counters. Zero preserves existing observations; new responses record
// absence explicitly and API projections return null for those fields.
type MetricMask uint8

const (
	MetricPlay MetricMask = 1 << iota
	MetricLike
	MetricComment
	MetricFavorite
	MetricForward
	InteractionMetrics = MetricLike | MetricComment | MetricFavorite | MetricForward
)

func MetricValue(value int, missing, metric MetricMask) *int {
	if missing&metric != 0 {
		return nil
	}
	if value < 0 {
		value = 0
	}
	return &value
}
