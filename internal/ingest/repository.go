package ingest

import (
	"context"
	"time"
)

type Repository interface {
	ListEvents(ctx context.Context, hubUserID string) ([]Event, error)
	AppendEvents(ctx context.Context, hubUserID string, events []Event) error
	ReplaceEvents(ctx context.Context, hubUserID string, events []Event) error
	ListVideoMetricObservations(ctx context.Context, hubUserID string) ([]VideoMetricObservation, error)
	AppendVideoMetricObservations(ctx context.Context, hubUserID string, observations []VideoMetricObservation) error
	ListRuns(ctx context.Context, hubUserID string) ([]Run, error)
	AppendRun(ctx context.Context, hubUserID string, run Run) error
	ListOperatorRuns(ctx context.Context, hubUserID string) ([]OperatorRun, error)
	AppendOperatorRun(ctx context.Context, hubUserID string, run OperatorRun) error
	ListBindings(ctx context.Context, hubUserID string) ([]SourceBinding, error)
	UpsertBinding(ctx context.Context, hubUserID string, binding SourceBinding) error
	DeleteBinding(ctx context.Context, hubUserID, source, targetKey string) error
	Reset(ctx context.Context, hubUserID string) error
}

// EventSummaryFacts is the bounded event projection needed to build an ingest
// summary without loading the complete persisted event set into the API.
type EventSummaryFacts struct {
	TotalEvents        int
	SourceCount        int
	AggregatedVideos   int
	AggregatedComments int
	AggregatedTopics   int
	LatestEvent        Event
	RecentEvents       []Event
	Sources            []EventSourceFacts
}

type EventSourceFacts struct {
	Source      string
	EventCount  int
	LatestEvent Event
}

// EventSummaryFactsProvider is implemented by stores that can aggregate exact
// summary totals while returning only the recent event evidence used by pages.
type EventSummaryFactsProvider interface {
	GetEventSummaryFacts(ctx context.Context, hubUserID string, recentLimit int) (EventSummaryFacts, error)
}

// SummaryRunsProvider returns the bounded run facts required by summary and
// binding-state views: recent runs plus each source's latest run/success/failure.
type SummaryRunsProvider interface {
	ListSummaryRuns(ctx context.Context, hubUserID string, recentLimit int) ([]Run, error)
}

// SourceAutoRunFact is the bounded per-source operator history needed by
// summary pages. Complete run and outcome history remains available through
// ListOperatorRuns.
type SourceAutoRunFact struct {
	Source        string
	LastRunStatus string
	LastRunAt     time.Time
	LastSuccessAt time.Time
	LastBatchID   string
}

type SourceAutoRunFactsProvider interface {
	GetSourceAutoRunFacts(ctx context.Context, hubUserID string) ([]SourceAutoRunFact, error)
}

// AccountMetricProvider is implemented by stores that can aggregate metrics
// for a bounded set of account identities without loading the full event log.
type AccountMetricProvider interface {
	ListAccountMetricRows(ctx context.Context, hubUserID string, channelUsernames []string) ([]AccountMetricRow, error)
}

// VideoHistoryProvider lets detail views fetch one video's evidence without
// materializing the complete metric observation table.
type VideoHistoryProvider interface {
	ListVideoHistory(ctx context.Context, hubUserID, objectID string) ([]Event, []VideoMetricObservation, error)
}
