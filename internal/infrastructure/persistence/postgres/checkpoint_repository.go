package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/duragraph/duragraph/internal/domain/checkpoint"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CheckpointRepository implements checkpoint.Repository using PostgreSQL
type CheckpointRepository struct {
	pool *pgxpool.Pool
}

// NewCheckpointRepository creates a new checkpoint repository
func NewCheckpointRepository(pool *pgxpool.Pool) *CheckpointRepository {
	return &CheckpointRepository{pool: pool}
}

// Save persists a checkpoint
func (r *CheckpointRepository) Save(ctx context.Context, cp *checkpoint.Checkpoint) error {
	channelValuesJSON, _ := json.Marshal(cp.ChannelValues())
	channelVersionsJSON, _ := json.Marshal(cp.ChannelVersions())
	versionsSeenJSON, _ := json.Marshal(cp.VersionsSeen())
	pendingSendsJSON, _ := json.Marshal(cp.PendingSends())

	query := `
		INSERT INTO checkpoints (
			id, thread_id, checkpoint_ns, checkpoint_id, parent_checkpoint_id,
			channel_values, channel_versions, versions_seen, pending_sends, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (thread_id, checkpoint_ns, checkpoint_id)
		DO UPDATE SET
			channel_values = $6,
			channel_versions = $7,
			versions_seen = $8,
			pending_sends = $9
	`

	_, err := r.pool.Exec(ctx, query,
		cp.ID(),
		cp.ThreadID(),
		cp.CheckpointNS(),
		cp.CheckpointID(),
		cp.ParentCheckpointID(),
		channelValuesJSON,
		channelVersionsJSON,
		versionsSeenJSON,
		pendingSendsJSON,
		cp.CreatedAt(),
	)

	return err
}

// FindByID retrieves a checkpoint by ID
func (r *CheckpointRepository) FindByID(ctx context.Context, id string) (*checkpoint.Checkpoint, error) {
	query := `
		SELECT id, thread_id, checkpoint_ns, checkpoint_id, parent_checkpoint_id,
			   channel_values, channel_versions, versions_seen, pending_sends, created_at
		FROM checkpoints
		WHERE id = $1
	`

	return r.scanCheckpoint(ctx, query, id)
}

// FindByCheckpointID retrieves a checkpoint by thread_id, checkpoint_ns, and checkpoint_id
func (r *CheckpointRepository) FindByCheckpointID(ctx context.Context, threadID, checkpointNS, checkpointID string) (*checkpoint.Checkpoint, error) {
	query := `
		SELECT id, thread_id, checkpoint_ns, checkpoint_id, parent_checkpoint_id,
			   channel_values, channel_versions, versions_seen, pending_sends, created_at
		FROM checkpoints
		WHERE thread_id = $1 AND checkpoint_ns = $2 AND checkpoint_id = $3
	`

	return r.scanCheckpoint(ctx, query, threadID, checkpointNS, checkpointID)
}

// FindLatest retrieves the latest checkpoint for a thread
func (r *CheckpointRepository) FindLatest(ctx context.Context, threadID string, checkpointNS string) (*checkpoint.Checkpoint, error) {
	query := `
		SELECT id, thread_id, checkpoint_ns, checkpoint_id, parent_checkpoint_id,
			   channel_values, channel_versions, versions_seen, pending_sends, created_at
		FROM checkpoints
		WHERE thread_id = $1 AND checkpoint_ns = $2
		ORDER BY created_at DESC
		LIMIT 1
	`

	return r.scanCheckpoint(ctx, query, threadID, checkpointNS)
}

// FindHistory retrieves checkpoint history for a thread
func (r *CheckpointRepository) FindHistory(ctx context.Context, threadID string, checkpointNS string, limit int, before string) ([]*checkpoint.Checkpoint, error) {
	var query string
	var args []interface{}

	if before != "" {
		query = `
			SELECT id, thread_id, checkpoint_ns, checkpoint_id, parent_checkpoint_id,
				   channel_values, channel_versions, versions_seen, pending_sends, created_at
			FROM checkpoints
			WHERE thread_id = $1 AND checkpoint_ns = $2 AND checkpoint_id < $3
			ORDER BY created_at DESC
			LIMIT $4
		`
		args = []interface{}{threadID, checkpointNS, before, limit}
	} else {
		query = `
			SELECT id, thread_id, checkpoint_ns, checkpoint_id, parent_checkpoint_id,
				   channel_values, channel_versions, versions_seen, pending_sends, created_at
			FROM checkpoints
			WHERE thread_id = $1 AND checkpoint_ns = $2
			ORDER BY created_at DESC
			LIMIT $3
		`
		args = []interface{}{threadID, checkpointNS, limit}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checkpoints []*checkpoint.Checkpoint
	for rows.Next() {
		cp, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		checkpoints = append(checkpoints, cp)
	}

	return checkpoints, nil
}

// Delete removes a checkpoint
func (r *CheckpointRepository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, "DELETE FROM checkpoints WHERE id = $1", id)
	return err
}

// SaveWrite persists a checkpoint write
func (r *CheckpointRepository) SaveWrite(ctx context.Context, write *checkpoint.CheckpointWrite) error {
	blobJSON, _ := json.Marshal(write.Blob())

	query := `
		INSERT INTO checkpoint_writes (
			id, thread_id, checkpoint_ns, checkpoint_id, task_id, idx, channel, type, blob, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err := r.pool.Exec(ctx, query,
		write.ID(),
		write.ThreadID(),
		write.CheckpointNS(),
		write.CheckpointID(),
		write.TaskID(),
		write.Idx(),
		write.Channel(),
		write.WriteType(),
		blobJSON,
		write.CreatedAt(),
	)

	return err
}

// FindWritesByCheckpoint retrieves all writes for a checkpoint
func (r *CheckpointRepository) FindWritesByCheckpoint(ctx context.Context, threadID, checkpointNS, checkpointID string) ([]*checkpoint.CheckpointWrite, error) {
	query := `
		SELECT id, thread_id, checkpoint_ns, checkpoint_id, task_id, idx, channel, type, blob, created_at
		FROM checkpoint_writes
		WHERE thread_id = $1 AND checkpoint_ns = $2 AND checkpoint_id = $3
		ORDER BY idx
	`

	rows, err := r.pool.Query(ctx, query, threadID, checkpointNS, checkpointID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var writes []*checkpoint.CheckpointWrite
	for rows.Next() {
		var id, tid, ns, cpid, taskID, channel, writeType string
		var idx int
		var blobJSON []byte
		var createdAt time.Time

		if err := rows.Scan(&id, &tid, &ns, &cpid, &taskID, &idx, &channel, &writeType, &blobJSON, &createdAt); err != nil {
			return nil, err
		}

		var blob map[string]interface{}
		json.Unmarshal(blobJSON, &blob)

		write := checkpoint.NewCheckpointWrite(tid, ns, cpid, taskID, idx, channel, writeType, blob)
		writes = append(writes, write)
	}

	return writes, nil
}

func (r *CheckpointRepository) scanCheckpoint(ctx context.Context, query string, args ...interface{}) (*checkpoint.Checkpoint, error) {
	row := r.pool.QueryRow(ctx, query, args...)
	return r.scanSingleRow(row)
}

type scannable interface {
	Scan(dest ...interface{}) error
}

func (r *CheckpointRepository) scanSingleRow(row scannable) (*checkpoint.Checkpoint, error) {
	var id, threadID, checkpointNS, checkpointID, parentCheckpointID string
	var channelValuesJSON, channelVersionsJSON, versionsSeenJSON, pendingSendsJSON []byte
	var createdAt time.Time

	err := row.Scan(
		&id,
		&threadID,
		&checkpointNS,
		&checkpointID,
		&parentCheckpointID,
		&channelValuesJSON,
		&channelVersionsJSON,
		&versionsSeenJSON,
		&pendingSendsJSON,
		&createdAt,
	)
	if err != nil {
		return nil, err
	}

	var channelValues map[string]interface{}
	var channelVersions map[string]int
	var versionsSeen map[string]map[string]int
	var pendingSends []map[string]interface{}

	json.Unmarshal(channelValuesJSON, &channelValues)
	json.Unmarshal(channelVersionsJSON, &channelVersions)
	json.Unmarshal(versionsSeenJSON, &versionsSeen)
	json.Unmarshal(pendingSendsJSON, &pendingSends)

	return checkpoint.Reconstitute(
		id,
		threadID,
		checkpointNS,
		checkpointID,
		parentCheckpointID,
		channelValues,
		channelVersions,
		versionsSeen,
		pendingSends,
		createdAt,
	), nil
}

func (r *CheckpointRepository) scanRow(rows scannable) (*checkpoint.Checkpoint, error) {
	return r.scanSingleRow(rows)
}
