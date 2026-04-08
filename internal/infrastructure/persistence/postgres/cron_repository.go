package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CronJob represents a scheduled cron job.
type CronJob struct {
	CronID         string
	AssistantID    string
	ThreadID       *string
	Schedule       string
	Timezone       string
	Payload        map[string]interface{}
	Metadata       map[string]interface{}
	Enabled        bool
	OnRunCompleted string
	EndTime        *time.Time
	NextRunDate    *time.Time
	UserID         *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// CronRepository provides CRUD operations for cron jobs.
type CronRepository struct {
	writePool *pgxpool.Pool
	readPool  *pgxpool.Pool
}

func NewCronRepository(pool *pgxpool.Pool) *CronRepository {
	return &CronRepository{writePool: pool, readPool: pool}
}

func NewCronRepositoryWithPools(writePool, readPool *pgxpool.Pool) *CronRepository {
	return &CronRepository{writePool: writePool, readPool: readPool}
}

const cronColumns = `cron_id, assistant_id, thread_id, schedule, timezone, payload, metadata, enabled, on_run_completed, end_time, next_run_date, user_id, created_at, updated_at`

func scanCron(row pgx.Row) (*CronJob, error) {
	var c CronJob
	err := row.Scan(
		&c.CronID, &c.AssistantID, &c.ThreadID, &c.Schedule, &c.Timezone,
		&c.Payload, &c.Metadata, &c.Enabled, &c.OnRunCompleted,
		&c.EndTime, &c.NextRunDate, &c.UserID, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func scanCrons(rows pgx.Rows) ([]CronJob, error) {
	var crons []CronJob
	for rows.Next() {
		var c CronJob
		if err := rows.Scan(
			&c.CronID, &c.AssistantID, &c.ThreadID, &c.Schedule, &c.Timezone,
			&c.Payload, &c.Metadata, &c.Enabled, &c.OnRunCompleted,
			&c.EndTime, &c.NextRunDate, &c.UserID, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		crons = append(crons, c)
	}
	return crons, nil
}

// Create inserts a new cron job and returns its ID.
func (r *CronRepository) Create(ctx context.Context, c *CronJob) (string, error) {
	query := `
		INSERT INTO crons (assistant_id, thread_id, schedule, timezone, payload, metadata, enabled, on_run_completed, end_time, next_run_date, user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING cron_id`

	var cronID string
	err := r.writePool.QueryRow(ctx, query,
		c.AssistantID, c.ThreadID, c.Schedule, c.Timezone, c.Payload, c.Metadata,
		c.Enabled, c.OnRunCompleted, c.EndTime, c.NextRunDate, c.UserID,
	).Scan(&cronID)
	if err != nil {
		return "", fmt.Errorf("failed to create cron: %w", err)
	}
	return cronID, nil
}

// GetByID retrieves a cron job by ID.
func (r *CronRepository) GetByID(ctx context.Context, cronID string) (*CronJob, error) {
	query := fmt.Sprintf(`SELECT %s FROM crons WHERE cron_id = $1`, cronColumns)
	cron, err := scanCron(r.readPool.QueryRow(ctx, query, cronID))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get cron: %w", err)
	}
	return cron, nil
}

// Delete removes a cron job by ID.
func (r *CronRepository) Delete(ctx context.Context, cronID string) error {
	_, err := r.writePool.Exec(ctx, `DELETE FROM crons WHERE cron_id = $1`, cronID)
	if err != nil {
		return fmt.Errorf("failed to delete cron: %w", err)
	}
	return nil
}

// Update patches a cron job. Only non-nil fields are updated.
func (r *CronRepository) Update(ctx context.Context, cronID string, updates map[string]interface{}) (*CronJob, error) {
	if len(updates) == 0 {
		return r.GetByID(ctx, cronID)
	}

	setClauses := ""
	args := []interface{}{}
	argIdx := 1

	for col, val := range updates {
		if setClauses != "" {
			setClauses += ", "
		}
		setClauses += fmt.Sprintf("%s = $%d", col, argIdx)
		args = append(args, val)
		argIdx++
	}

	setClauses += fmt.Sprintf(", updated_at = $%d", argIdx)
	args = append(args, time.Now())
	argIdx++

	args = append(args, cronID)
	query := fmt.Sprintf(`UPDATE crons SET %s WHERE cron_id = $%d RETURNING %s`, setClauses, argIdx, cronColumns)

	cron, err := scanCron(r.writePool.QueryRow(ctx, query, args...))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update cron: %w", err)
	}
	return cron, nil
}

// Search finds cron jobs matching optional filters with pagination.
func (r *CronRepository) Search(ctx context.Context, assistantID, threadID *string, enabled *bool, limit, offset int, sortBy, sortOrder string) ([]CronJob, error) {
	if limit <= 0 {
		limit = 10
	}

	query := fmt.Sprintf(`SELECT %s FROM crons WHERE 1=1`, cronColumns)
	args := []interface{}{}
	argIdx := 1

	if assistantID != nil {
		query += fmt.Sprintf(" AND assistant_id = $%d", argIdx)
		args = append(args, *assistantID)
		argIdx++
	}
	if threadID != nil {
		query += fmt.Sprintf(" AND thread_id = $%d", argIdx)
		args = append(args, *threadID)
		argIdx++
	}
	if enabled != nil {
		query += fmt.Sprintf(" AND enabled = $%d", argIdx)
		args = append(args, *enabled)
		argIdx++
	}

	validSortColumns := map[string]bool{
		"cron_id": true, "assistant_id": true, "thread_id": true,
		"created_at": true, "updated_at": true, "next_run_date": true, "end_time": true,
	}
	if !validSortColumns[sortBy] {
		sortBy = "created_at"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	query += fmt.Sprintf(" ORDER BY %s %s LIMIT $%d OFFSET $%d", sortBy, sortOrder, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := r.readPool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search crons: %w", err)
	}
	defer rows.Close()

	return scanCrons(rows)
}

// Count returns the number of crons matching optional filters.
func (r *CronRepository) Count(ctx context.Context, assistantID, threadID *string) (int, error) {
	query := `SELECT COUNT(*) FROM crons WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if assistantID != nil {
		query += fmt.Sprintf(" AND assistant_id = $%d", argIdx)
		args = append(args, *assistantID)
		argIdx++
	}
	if threadID != nil {
		query += fmt.Sprintf(" AND thread_id = $%d", argIdx)
		args = append(args, *threadID)
		argIdx++
	}

	var count int
	err := r.readPool.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count crons: %w", err)
	}
	return count, nil
}

// GetDueJobs returns enabled cron jobs whose next_run_date has passed.
func (r *CronRepository) GetDueJobs(ctx context.Context, now time.Time) ([]CronJob, error) {
	query := fmt.Sprintf(`
		SELECT %s FROM crons
		WHERE enabled = TRUE
		  AND next_run_date IS NOT NULL
		  AND next_run_date <= $1
		  AND (end_time IS NULL OR end_time > $1)
		ORDER BY next_run_date ASC
		LIMIT 100`, cronColumns)

	rows, err := r.readPool.Query(ctx, query, now)
	if err != nil {
		return nil, fmt.Errorf("failed to get due cron jobs: %w", err)
	}
	defer rows.Close()

	return scanCrons(rows)
}

// UpdateNextRun sets the next_run_date for a cron job.
func (r *CronRepository) UpdateNextRun(ctx context.Context, cronID string, nextRun time.Time) error {
	_, err := r.writePool.Exec(ctx,
		`UPDATE crons SET next_run_date = $1, updated_at = NOW() WHERE cron_id = $2`,
		nextRun, cronID)
	if err != nil {
		return fmt.Errorf("failed to update next run: %w", err)
	}
	return nil
}
