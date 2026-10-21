package ingestpg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
)

type CommentRepository struct {
	db *sql.DB
}

var _ ingest.RawCommentRepository = (*CommentRepository)(nil)
var _ ingest.VideoCommentSyncRepository = (*CommentRepository)(nil)

func NewCommentRepository(client *postgres.Client) (*CommentRepository, error) {
	if client == nil || client.DB() == nil {
		return nil, errors.New("postgres client is required")
	}
	if err := postgres.EnsureCommentSchema(client.DB()); err != nil {
		return nil, err
	}
	return &CommentRepository{db: client.DB()}, nil
}

func (r *CommentRepository) Upsert(ctx context.Context, hubUserID string, items []ingest.RawCommentRecord) error {
	if ctx == nil {
		ctx = context.Background()
	}
	hubUserID = strings.TrimSpace(hubUserID)
	if hubUserID == "" {
		return errors.New("comment hub user id is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	for _, item := range items {
		if strings.TrimSpace(item.VideoObjectID) == "" || strings.TrimSpace(item.CommentID) == "" {
			continue
		}
		rawPayload := item.RawPayload
		if len(rawPayload) == 0 {
			rawPayload = []byte(`{}`)
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO insight_comment_records (
  hub_user_id, comment_id, video_object_id, video_object_nonce_id, account_id,
  video_title, channel_name, channel_username, parent_comment_id,
  author_username, author_nickname, content, ip_region, created_at,
  like_count, reply_count, collected_at, source, raw_payload
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19::jsonb)
ON CONFLICT (hub_user_id, video_object_id, comment_id) DO UPDATE SET
  video_object_nonce_id = EXCLUDED.video_object_nonce_id,
  account_id = EXCLUDED.account_id,
  video_title = EXCLUDED.video_title,
  channel_name = EXCLUDED.channel_name,
  channel_username = EXCLUDED.channel_username,
  parent_comment_id = EXCLUDED.parent_comment_id,
  author_username = EXCLUDED.author_username,
  author_nickname = EXCLUDED.author_nickname,
  content = EXCLUDED.content,
  ip_region = EXCLUDED.ip_region,
  created_at = EXCLUDED.created_at,
  like_count = EXCLUDED.like_count,
  reply_count = EXCLUDED.reply_count,
  collected_at = EXCLUDED.collected_at,
  source = EXCLUDED.source,
  raw_payload = EXCLUDED.raw_payload`,
			hubUserID, item.CommentID, item.VideoObjectID, item.VideoObjectNonceID, item.AccountID,
			item.VideoTitle, item.ChannelName, item.ChannelUsername, item.ParentCommentID,
			item.AuthorUsername, item.AuthorNickname, item.Content, item.IPRegion, nullableTime(item.CreatedAt),
			item.LikeCount, item.ReplyCount, item.CollectedAt, item.Source, string(rawPayload))
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *CommentRepository) List(ctx context.Context, hubUserID string, query ingest.RawCommentQuery) (ingest.RawCommentPage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	hubUserID = strings.TrimSpace(hubUserID)
	query = ingest.NormalizeRawCommentQuery(query)
	whereSQL, args := rawCommentWhere(hubUserID, query)
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM insight_comment_records WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return ingest.RawCommentPage{}, err
	}
	totalPages := (total + query.PageSize - 1) / query.PageSize
	if totalPages == 0 {
		totalPages = 1
	}
	if query.Page > totalPages {
		query.Page = totalPages
	}
	listArgs := append(append([]any(nil), args...), query.PageSize, (query.Page-1)*query.PageSize)
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
SELECT comment_id, video_object_id, video_object_nonce_id, account_id,
       video_title, channel_name, channel_username, parent_comment_id,
       author_username, author_nickname, content, ip_region, created_at,
       like_count, reply_count, collected_at, source
FROM insight_comment_records
WHERE %s
ORDER BY COALESCE(created_at, collected_at) DESC,
         collected_at DESC, video_object_id DESC, comment_id DESC
LIMIT $%d OFFSET $%d`, whereSQL, len(args)+1, len(args)+2), listArgs...)
	if err != nil {
		return ingest.RawCommentPage{}, err
	}
	defer rows.Close()
	items := []ingest.RawCommentRecord{}
	for rows.Next() {
		var item ingest.RawCommentRecord
		var createdAt sql.NullTime
		if err := rows.Scan(&item.CommentID, &item.VideoObjectID, &item.VideoObjectNonceID, &item.AccountID,
			&item.VideoTitle, &item.ChannelName, &item.ChannelUsername, &item.ParentCommentID,
			&item.AuthorUsername, &item.AuthorNickname, &item.Content, &item.IPRegion, &createdAt,
			&item.LikeCount, &item.ReplyCount, &item.CollectedAt, &item.Source); err != nil {
			return ingest.RawCommentPage{}, err
		}
		item.HubUserID = hubUserID
		item.ID = item.VideoObjectID + "\x00" + item.CommentID
		if createdAt.Valid {
			item.CreatedAt = createdAt.Time
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ingest.RawCommentPage{}, err
	}
	return ingest.RawCommentPage{Items: items, Page: query.Page, PageSize: query.PageSize, Total: total, TotalPages: totalPages}, nil
}

func (r *CommentRepository) ExportJSON(ctx context.Context, hubUserID string, query ingest.RawCommentQuery, dst io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	hubUserID = strings.TrimSpace(hubUserID)
	whereSQL, args := rawCommentWhere(hubUserID, query)
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM insight_comment_records WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return err
	}
	writer, err := ingest.NewRawCommentJSONWriter(dst, ingest.RawCommentExportMetadata{
		HubUserID: hubUserID, Total: total,
	}, query)
	if err != nil {
		return err
	}
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
SELECT comment_id, video_object_id, video_object_nonce_id, account_id,
       video_title, channel_name, channel_username, parent_comment_id,
       author_username, author_nickname, content, ip_region, created_at,
       like_count, reply_count, collected_at, source, raw_payload
FROM insight_comment_records
WHERE %s
ORDER BY COALESCE(created_at, collected_at) DESC,
         collected_at DESC, video_object_id DESC, comment_id DESC`, whereSQL), args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item ingest.RawCommentRecord
		var createdAt sql.NullTime
		var rawPayload []byte
		if err := rows.Scan(&item.CommentID, &item.VideoObjectID, &item.VideoObjectNonceID, &item.AccountID,
			&item.VideoTitle, &item.ChannelName, &item.ChannelUsername, &item.ParentCommentID,
			&item.AuthorUsername, &item.AuthorNickname, &item.Content, &item.IPRegion, &createdAt,
			&item.LikeCount, &item.ReplyCount, &item.CollectedAt, &item.Source, &rawPayload); err != nil {
			return err
		}
		item.HubUserID = hubUserID
		item.ID = item.VideoObjectID + "\x00" + item.CommentID
		if createdAt.Valid {
			item.CreatedAt = createdAt.Time
		}
		if len(rawPayload) > 0 {
			item.RawPayload = append(json.RawMessage(nil), rawPayload...)
		}
		if err := writer.Write(item); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return writer.Close()
}

func rawCommentWhere(hubUserID string, query ingest.RawCommentQuery) (string, []any) {
	conditions := []string{"hub_user_id = $1"}
	args := []any{hubUserID}
	add := func(condition string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(condition, len(args)))
	}
	if value := strings.TrimSpace(query.ObjectID); value != "" {
		add("video_object_id = $%d", value)
	}
	if value := strings.TrimSpace(query.AccountID); value != "" {
		add("account_id = $%d", value)
	}
	if value := strings.ToLower(strings.TrimSpace(query.AccountKeyword)); value != "" {
		pattern := "%" + value + "%"
		args = append(args, pattern)
		placeholder := fmt.Sprintf("$%d", len(args))
		conditions = append(conditions, "("+
			"LOWER(account_id) LIKE "+placeholder+" OR "+
			"LOWER(channel_name) LIKE "+placeholder+" OR "+
			"LOWER(channel_username) LIKE "+placeholder+")")
	}
	if value := strings.TrimSpace(query.Source); value != "" {
		add("source = $%d", value)
	}
	for _, rawID := range query.ExcludeIDs {
		parts := strings.SplitN(strings.TrimSpace(rawID), "\x00", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			continue
		}
		videoPlaceholder := len(args) + 1
		commentPlaceholder := len(args) + 2
		args = append(args, parts[0], parts[1])
		conditions = append(conditions, fmt.Sprintf("NOT (video_object_id = $%d AND comment_id = $%d)", videoPlaceholder, commentPlaceholder))
	}
	if !query.From.IsZero() {
		add("collected_at >= $%d", query.From)
	}
	if !query.To.IsZero() {
		add("collected_at <= $%d", query.To)
	}
	if value := strings.ToLower(strings.TrimSpace(query.Keyword)); value != "" {
		pattern := "%" + value + "%"
		args = append(args, pattern)
		placeholder := fmt.Sprintf("$%d", len(args))
		conditions = append(conditions, "("+
			"LOWER(comment_id) LIKE "+placeholder+" OR "+
			"LOWER(video_object_id) LIKE "+placeholder+" OR "+
			"LOWER(author_username) LIKE "+placeholder+" OR "+
			"LOWER(author_nickname) LIKE "+placeholder+" OR "+
			"LOWER(content) LIKE "+placeholder+" OR "+
			"LOWER(video_title) LIKE "+placeholder+" OR "+
			"LOWER(channel_name) LIKE "+placeholder+" OR "+
			"LOWER(ip_region) LIKE "+placeholder+")")
	}
	return strings.Join(conditions, " AND "), args
}

func (r *CommentRepository) GetState(ctx context.Context, hubUserID, accountID, videoObjectID string) (ingest.VideoCommentSyncState, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var state ingest.VideoCommentSyncState
	var lastObservedAt, lastCollectedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `
SELECT hub_user_id, account_id, video_object_id, video_object_nonce_id,
       last_comment_count, last_observed_at, last_collected_at, status, last_error
FROM insight_video_comment_sync
WHERE hub_user_id = $1 AND account_id = $2 AND video_object_id = $3`, hubUserID, accountID, videoObjectID).
		Scan(&state.HubUserID, &state.AccountID, &state.VideoObjectID, &state.VideoObjectNonceID,
			&state.LastCommentCount, &lastObservedAt, &lastCollectedAt, &state.Status, &state.LastError)
	if errors.Is(err, sql.ErrNoRows) {
		return ingest.VideoCommentSyncState{}, false, nil
	}
	if err != nil {
		return ingest.VideoCommentSyncState{}, false, err
	}
	if lastObservedAt.Valid {
		state.LastObservedAt = lastObservedAt.Time
	}
	if lastCollectedAt.Valid {
		state.LastCollectedAt = lastCollectedAt.Time
	}
	return state, true, nil
}

func (r *CommentRepository) UpsertState(ctx context.Context, state ingest.VideoCommentSyncState) error {
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO insight_video_comment_sync (
  hub_user_id, account_id, video_object_id, video_object_nonce_id,
  last_comment_count, last_observed_at, last_collected_at, status, last_error
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (hub_user_id, account_id, video_object_id) DO UPDATE SET
  video_object_nonce_id = EXCLUDED.video_object_nonce_id,
  last_comment_count = EXCLUDED.last_comment_count,
  last_observed_at = EXCLUDED.last_observed_at,
  last_collected_at = EXCLUDED.last_collected_at,
  status = EXCLUDED.status,
  last_error = EXCLUDED.last_error`,
		state.HubUserID, state.AccountID, state.VideoObjectID, state.VideoObjectNonceID,
		state.LastCommentCount, nullableTime(state.LastObservedAt), nullableTime(state.LastCollectedAt), state.Status, state.LastError)
	return err
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}
