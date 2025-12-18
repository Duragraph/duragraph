package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/duragraph/duragraph/internal/domain/workflow"
	"github.com/duragraph/duragraph/internal/pkg/errors"
	pkguuid "github.com/duragraph/duragraph/internal/pkg/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ThreadRepository implements the workflow.ThreadRepository interface
type ThreadRepository struct {
	pool       *pgxpool.Pool
	eventStore *EventStore
}

// NewThreadRepository creates a new thread repository
func NewThreadRepository(pool *pgxpool.Pool, eventStore *EventStore) *ThreadRepository {
	return &ThreadRepository{
		pool:       pool,
		eventStore: eventStore,
	}
}

// Save persists a thread aggregate and its events
func (r *ThreadRepository) Save(ctx context.Context, thread *workflow.Thread) error {
	// Save thread to CRUD table
	metadataJSON, _ := json.Marshal(thread.Metadata())

	_, err := r.pool.Exec(ctx, `
		INSERT INTO threads (id, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4)
	`,
		thread.ID(),
		metadataJSON,
		thread.CreatedAt(),
		thread.UpdatedAt(),
	)

	if err != nil {
		return fmt.Errorf("failed to save thread: %w", err)
	}

	// Save messages to messages table
	for _, msg := range thread.Messages() {
		msgMetadataJSON, _ := json.Marshal(msg.Metadata)
		_, err = r.pool.Exec(ctx, `
			INSERT INTO messages (id, thread_id, role, content, metadata, created_at)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (id) DO NOTHING
		`,
			msg.ID,
			thread.ID(),
			msg.Role,
			msg.Content,
			msgMetadataJSON,
			msg.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to save message: %w", err)
		}
	}

	err = nil

	if err != nil {
		return errors.Internal("failed to save thread", err)
	}

	// Save events to event store
	if len(thread.Events()) > 0 {
		streamID := pkguuid.New()
		if err := r.eventStore.SaveEvents(ctx, streamID, "thread", thread.ID(), thread.Events()); err != nil {
			return err
		}

		// Clear events after saving
		thread.ClearEvents()
	}

	return nil
}

// FindByID retrieves a thread by ID
func (r *ThreadRepository) FindByID(ctx context.Context, id string) (*workflow.Thread, error) {
	var threadID string
	var metadataJSON []byte
	var createdAt, updatedAt time.Time

	err := r.pool.QueryRow(ctx, `
		SELECT id, metadata, created_at, updated_at
		FROM threads
		WHERE id = $1
	`, id).Scan(
		&threadID, &metadataJSON, &createdAt, &updatedAt,
	)

	if err != nil {
		return nil, errors.NotFound("thread", id)
	}

	// Reconstruct thread metadata
	var metadata map[string]interface{}
	json.Unmarshal(metadataJSON, &metadata)

	// Load messages from messages table
	rows, err := r.pool.Query(ctx, `
		SELECT id, role, content, metadata, created_at
		FROM messages
		WHERE thread_id = $1
		ORDER BY created_at ASC
	`, threadID)

	if err != nil {
		return nil, fmt.Errorf("failed to load messages: %w", err)
	}
	defer rows.Close()

	messages := make([]workflow.Message, 0)
	for rows.Next() {
		var msgID, role, content string
		var msgMetadataJSON []byte
		var msgCreatedAt time.Time

		err := rows.Scan(&msgID, &role, &content, &msgMetadataJSON, &msgCreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}

		var msgMetadata map[string]interface{}
		json.Unmarshal(msgMetadataJSON, &msgMetadata)

		messages = append(messages, workflow.Message{
			ID:        msgID,
			Role:      role,
			Content:   content,
			Metadata:  msgMetadata,
			CreatedAt: msgCreatedAt,
		})
	}

	// Reconstruct thread with database values
	thread, err := workflow.ReconstructThread(
		threadID, messages, metadata, createdAt, updatedAt,
	)
	if err != nil {
		return nil, err
	}

	return thread, nil
}

// List retrieves threads with pagination
func (r *ThreadRepository) List(ctx context.Context, limit, offset int) ([]*workflow.Thread, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, metadata, created_at, updated_at
		FROM threads
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)

	if err != nil {
		return nil, errors.Internal("failed to query threads", err)
	}
	defer rows.Close()

	threads := make([]*workflow.Thread, 0)

	for rows.Next() {
		var threadID string
		var metadataJSON []byte
		var createdAt, updatedAt time.Time

		err := rows.Scan(&threadID, &metadataJSON, &createdAt, &updatedAt)
		if err != nil {
			return nil, errors.Internal("failed to scan thread", err)
		}

		var metadata map[string]interface{}
		json.Unmarshal(metadataJSON, &metadata)

		// Load messages for this thread
		msgRows, err := r.pool.Query(ctx, `
			SELECT id, role, content, metadata, created_at
			FROM messages
			WHERE thread_id = $1
			ORDER BY created_at ASC
		`, threadID)

		messages := make([]workflow.Message, 0)
		if err == nil {
			defer msgRows.Close()
			for msgRows.Next() {
				var msgID, role, content string
				var msgMetadataJSON []byte
				var msgCreatedAt time.Time

				if err := msgRows.Scan(&msgID, &role, &content, &msgMetadataJSON, &msgCreatedAt); err == nil {
					var msgMetadata map[string]interface{}
					json.Unmarshal(msgMetadataJSON, &msgMetadata)

					messages = append(messages, workflow.Message{
						ID:        msgID,
						Role:      role,
						Content:   content,
						Metadata:  msgMetadata,
						CreatedAt: msgCreatedAt,
					})
				}
			}
		}

		thread, _ := workflow.ReconstructThread(
			threadID, messages, metadata, createdAt, updatedAt,
		)
		threads = append(threads, thread)
	}

	return threads, nil
}

// Update updates an existing thread
func (r *ThreadRepository) Update(ctx context.Context, thread *workflow.Thread) error {
	metadataJSON, _ := json.Marshal(thread.Metadata())

	_, err := r.pool.Exec(ctx, `
		UPDATE threads
		SET metadata = $1, updated_at = $2
		WHERE id = $3
	`,
		metadataJSON,
		thread.UpdatedAt(),
		thread.ID(),
	)

	// Update messages in messages table if needed
	// This is a simplified approach - in production would track which messages are new
	for _, msg := range thread.Messages() {
		msgMetadataJSON, _ := json.Marshal(msg.Metadata)
		_, err = r.pool.Exec(ctx, `
			INSERT INTO messages (id, thread_id, role, content, metadata, created_at)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (id) DO NOTHING
		`,
			msg.ID,
			thread.ID(),
			msg.Role,
			msg.Content,
			msgMetadataJSON,
			msg.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to save message: %w", err)
		}
	}

	if err != nil {
		return errors.Internal("failed to update thread", err)
	}

	// Save events to event store
	if len(thread.Events()) > 0 {
		streamID := pkguuid.New()
		if err := r.eventStore.SaveEvents(ctx, streamID, "thread", thread.ID(), thread.Events()); err != nil {
			return err
		}

		thread.ClearEvents()
	}

	return nil
}

// Delete removes a thread
func (r *ThreadRepository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM threads WHERE id = $1`, id)
	if err != nil {
		return errors.Internal("failed to delete thread", err)
	}
	return nil
}

// Search retrieves threads matching the given filters
func (r *ThreadRepository) Search(ctx context.Context, filters workflow.ThreadSearchFilters) ([]*workflow.Thread, error) {
	query := `
		SELECT id, metadata, created_at, updated_at
		FROM threads
		WHERE 1=1
	`
	args := make([]interface{}, 0)
	argIdx := 1

	if filters.Status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, filters.Status)
		argIdx++
	}

	if filters.Metadata != nil {
		metadataJSON, _ := json.Marshal(filters.Metadata)
		query += fmt.Sprintf(" AND metadata @> $%d", argIdx)
		args = append(args, metadataJSON)
		argIdx++
	}

	query += ` ORDER BY created_at DESC`

	if filters.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIdx)
		args = append(args, filters.Limit)
		argIdx++
	}

	if filters.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argIdx)
		args = append(args, filters.Offset)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, errors.Internal("failed to search threads", err)
	}
	defer rows.Close()

	threads := make([]*workflow.Thread, 0)

	for rows.Next() {
		var threadID string
		var metadataJSON []byte
		var createdAt, updatedAt time.Time

		err := rows.Scan(&threadID, &metadataJSON, &createdAt, &updatedAt)
		if err != nil {
			return nil, errors.Internal("failed to scan thread", err)
		}

		var metadata map[string]interface{}
		json.Unmarshal(metadataJSON, &metadata)

		thread, _ := workflow.ReconstructThread(
			threadID, nil, metadata, createdAt, updatedAt,
		)
		threads = append(threads, thread)
	}

	return threads, nil
}

// Count returns the number of threads matching the given filters
func (r *ThreadRepository) Count(ctx context.Context, filters workflow.ThreadSearchFilters) (int, error) {
	query := `SELECT COUNT(*) FROM threads WHERE 1=1`
	args := make([]interface{}, 0)
	argIdx := 1

	if filters.Status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, filters.Status)
		argIdx++
	}

	if filters.Metadata != nil {
		metadataJSON, _ := json.Marshal(filters.Metadata)
		query += fmt.Sprintf(" AND metadata @> $%d", argIdx)
		args = append(args, metadataJSON)
	}

	var count int
	err := r.pool.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, errors.Internal("failed to count threads", err)
	}

	return count, nil
}
