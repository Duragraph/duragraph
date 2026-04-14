package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/duragraph/duragraph/internal/domain/worker"
	"github.com/duragraph/duragraph/internal/pkg/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WorkerRepository implements worker.Repository using PostgreSQL.
type WorkerRepository struct {
	pool *pgxpool.Pool
}

// NewWorkerRepository creates a new PostgreSQL-backed worker repository.
func NewWorkerRepository(pool *pgxpool.Pool) *WorkerRepository {
	return &WorkerRepository{pool: pool}
}

func (r *WorkerRepository) Save(ctx context.Context, w *worker.Worker) error {
	capJSON, _ := json.Marshal(w.Capabilities)
	graphDefsJSON, _ := json.Marshal(w.GraphDefinitions)

	_, err := r.pool.Exec(ctx, `
		INSERT INTO workers (id, name, status, capabilities, graph_definitions,
		                     active_runs, total_runs, failed_runs, last_heartbeat_at, registered_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			status = EXCLUDED.status,
			capabilities = EXCLUDED.capabilities,
			graph_definitions = EXCLUDED.graph_definitions,
			active_runs = EXCLUDED.active_runs,
			total_runs = EXCLUDED.total_runs,
			failed_runs = EXCLUDED.failed_runs,
			last_heartbeat_at = EXCLUDED.last_heartbeat_at
	`,
		w.ID, w.Name, string(w.Status), capJSON, graphDefsJSON,
		w.ActiveRuns, w.TotalRuns, w.FailedRuns, w.LastHeartbeat, w.RegisteredAt,
	)
	if err != nil {
		return errors.Internal("failed to save worker", err)
	}
	return nil
}

func (r *WorkerRepository) FindByID(ctx context.Context, id string) (*worker.Worker, error) {
	var w worker.Worker
	var statusStr string
	var capJSON, graphDefsJSON []byte

	err := r.pool.QueryRow(ctx, `
		SELECT id, name, status, capabilities, graph_definitions,
		       active_runs, total_runs, failed_runs, last_heartbeat_at, registered_at
		FROM workers WHERE id = $1
	`, id).Scan(
		&w.ID, &w.Name, &statusStr, &capJSON, &graphDefsJSON,
		&w.ActiveRuns, &w.TotalRuns, &w.FailedRuns, &w.LastHeartbeat, &w.RegisteredAt,
	)
	if err != nil {
		return nil, errors.NotFound("worker", id)
	}

	w.Status = worker.Status(statusStr)
	json.Unmarshal(capJSON, &w.Capabilities)
	json.Unmarshal(graphDefsJSON, &w.GraphDefinitions)

	return &w, nil
}

func (r *WorkerRepository) FindAll(ctx context.Context) ([]*worker.Worker, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, status, capabilities, graph_definitions,
		       active_runs, total_runs, failed_runs, last_heartbeat_at, registered_at
		FROM workers ORDER BY registered_at DESC
	`)
	if err != nil {
		return nil, errors.Internal("failed to query workers", err)
	}
	defer rows.Close()

	return r.scanWorkers(rows)
}

func (r *WorkerRepository) FindHealthy(ctx context.Context, threshold time.Duration) ([]*worker.Worker, error) {
	cutoff := time.Now().Add(-threshold)
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, status, capabilities, graph_definitions,
		       active_runs, total_runs, failed_runs, last_heartbeat_at, registered_at
		FROM workers
		WHERE last_heartbeat_at > $1 AND status != 'offline'
		ORDER BY active_runs ASC
	`, cutoff)
	if err != nil {
		return nil, errors.Internal("failed to query healthy workers", err)
	}
	defer rows.Close()

	return r.scanWorkers(rows)
}

func (r *WorkerRepository) FindForGraph(ctx context.Context, graphID string, threshold time.Duration) (*worker.Worker, error) {
	cutoff := time.Now().Add(-threshold)

	var w worker.Worker
	var statusStr string
	var capJSON, graphDefsJSON []byte

	err := r.pool.QueryRow(ctx, `
		SELECT id, name, status, capabilities, graph_definitions,
		       active_runs, total_runs, failed_runs, last_heartbeat_at, registered_at
		FROM workers
		WHERE last_heartbeat_at > $1
		  AND status != 'offline'
		  AND capabilities->'graphs' ? $2
		  AND active_runs < GREATEST(COALESCE((capabilities->>'max_concurrent_runs')::int, 10), 1)
		ORDER BY active_runs ASC
		LIMIT 1
	`, cutoff, graphID).Scan(
		&w.ID, &w.Name, &statusStr, &capJSON, &graphDefsJSON,
		&w.ActiveRuns, &w.TotalRuns, &w.FailedRuns, &w.LastHeartbeat, &w.RegisteredAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		fmt.Printf("FindForGraph query error (graphID=%s): %v\n", graphID, err)
		return nil, errors.Internal("failed to find worker for graph", err)
	}

	w.Status = worker.Status(statusStr)
	json.Unmarshal(capJSON, &w.Capabilities)
	json.Unmarshal(graphDefsJSON, &w.GraphDefinitions)

	return &w, nil
}

func (r *WorkerRepository) Heartbeat(ctx context.Context, id string, status worker.Status, activeRuns, totalRuns, failedRuns int) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE workers
		SET status = $2, active_runs = $3, total_runs = $4, failed_runs = $5,
		    last_heartbeat_at = NOW()
		WHERE id = $1
	`, id, string(status), activeRuns, totalRuns, failedRuns)
	if err != nil {
		return errors.Internal("failed to update worker heartbeat", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.NotFound("worker", id)
	}
	return nil
}

func (r *WorkerRepository) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM workers WHERE id = $1`, id)
	if err != nil {
		return errors.Internal("failed to delete worker", err)
	}
	return nil
}

func (r *WorkerRepository) CleanupStale(ctx context.Context, threshold time.Duration) (int, error) {
	cutoff := time.Now().Add(-threshold)
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM workers WHERE last_heartbeat_at < $1
	`, cutoff)
	if err != nil {
		return 0, errors.Internal("failed to cleanup stale workers", err)
	}
	return int(tag.RowsAffected()), nil
}

func (r *WorkerRepository) FindGraphDefinition(ctx context.Context, graphID string) (*worker.GraphDefinition, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT graph_definitions FROM workers
		WHERE status != 'offline'
		ORDER BY last_heartbeat_at DESC
	`)
	if err != nil {
		return nil, errors.Internal("failed to query graph definitions", err)
	}
	defer rows.Close()

	for rows.Next() {
		var graphDefsJSON []byte
		if err := rows.Scan(&graphDefsJSON); err != nil {
			continue
		}

		var defs []worker.GraphDefinition
		if err := json.Unmarshal(graphDefsJSON, &defs); err != nil {
			continue
		}

		for _, d := range defs {
			if d.GraphID == graphID {
				return &d, nil
			}
		}
	}

	return nil, errors.NotFound("graph_definition", graphID)
}

type pgxRows interface {
	Next() bool
	Scan(dest ...interface{}) error
	Close()
}

func (r *WorkerRepository) scanWorkers(rows pgxRows) ([]*worker.Worker, error) {
	var workers []*worker.Worker
	for rows.Next() {
		var w worker.Worker
		var statusStr string
		var capJSON, graphDefsJSON []byte

		if err := rows.Scan(
			&w.ID, &w.Name, &statusStr, &capJSON, &graphDefsJSON,
			&w.ActiveRuns, &w.TotalRuns, &w.FailedRuns, &w.LastHeartbeat, &w.RegisteredAt,
		); err != nil {
			return nil, errors.Internal("failed to scan worker", err)
		}

		w.Status = worker.Status(statusStr)
		json.Unmarshal(capJSON, &w.Capabilities)
		json.Unmarshal(graphDefsJSON, &w.GraphDefinitions)

		workers = append(workers, &w)
	}
	return workers, nil
}
