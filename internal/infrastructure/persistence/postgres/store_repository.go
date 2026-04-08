package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StoreItem represents a namespaced key-value item.
type StoreItem struct {
	Namespace []string
	Key       string
	Value     map[string]interface{}
	CreatedAt time.Time
	UpdatedAt time.Time
}

// StoreRepository provides CRUD operations for the namespaced key-value store.
type StoreRepository struct {
	writePool *pgxpool.Pool
	readPool  *pgxpool.Pool
}

func NewStoreRepository(pool *pgxpool.Pool) *StoreRepository {
	return &StoreRepository{writePool: pool, readPool: pool}
}

func NewStoreRepositoryWithPools(writePool, readPool *pgxpool.Pool) *StoreRepository {
	return &StoreRepository{writePool: writePool, readPool: readPool}
}

// Put inserts or updates an item. ttlMinutes <= 0 means no expiration.
func (r *StoreRepository) Put(ctx context.Context, namespace []string, key string, value map[string]interface{}, ttlMinutes int) error {
	var expiresAt *time.Time
	if ttlMinutes > 0 {
		t := time.Now().Add(time.Duration(ttlMinutes) * time.Minute)
		expiresAt = &t
	}

	query := `
		INSERT INTO store_items (namespace, key, value, expires_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (namespace, key)
		DO UPDATE SET value = $3, updated_at = NOW(), expires_at = $4`

	_, err := r.writePool.Exec(ctx, query, namespace, key, value, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to put store item: %w", err)
	}
	return nil
}

// Get retrieves a single item by namespace and key. Returns nil if not found or expired.
func (r *StoreRepository) Get(ctx context.Context, namespace []string, key string, refreshTTL bool) (*StoreItem, error) {
	query := `
		SELECT namespace, key, value, created_at, updated_at
		FROM store_items
		WHERE namespace = $1 AND key = $2
		  AND (expires_at IS NULL OR expires_at > NOW())`

	row := r.readPool.QueryRow(ctx, query, namespace, key)

	var item StoreItem
	err := row.Scan(&item.Namespace, &item.Key, &item.Value, &item.CreatedAt, &item.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get store item: %w", err)
	}

	if refreshTTL {
		_, _ = r.writePool.Exec(ctx,
			`UPDATE store_items SET expires_at = expires_at + (expires_at - updated_at), updated_at = NOW()
			 WHERE namespace = $1 AND key = $2 AND expires_at IS NOT NULL`,
			namespace, key)
	}

	return &item, nil
}

// Delete removes an item by namespace and key.
func (r *StoreRepository) Delete(ctx context.Context, namespace []string, key string) error {
	_, err := r.writePool.Exec(ctx,
		`DELETE FROM store_items WHERE namespace = $1 AND key = $2`,
		namespace, key)
	if err != nil {
		return fmt.Errorf("failed to delete store item: %w", err)
	}
	return nil
}

// Search finds items within a namespace prefix, with optional value filter, limit, and offset.
func (r *StoreRepository) Search(ctx context.Context, namespacePrefix []string, filter map[string]interface{}, limit, offset int) ([]StoreItem, error) {
	if limit <= 0 {
		limit = 10
	}

	query := `
		SELECT namespace, key, value, created_at, updated_at
		FROM store_items
		WHERE namespace[1:$1] = $2
		  AND (expires_at IS NULL OR expires_at > NOW())`

	args := []interface{}{len(namespacePrefix), namespacePrefix}
	argIdx := 3

	if len(filter) > 0 {
		query += fmt.Sprintf(" AND value @> $%d", argIdx)
		args = append(args, filter)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY updated_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := r.readPool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search store items: %w", err)
	}
	defer rows.Close()

	var items []StoreItem
	for rows.Next() {
		var item StoreItem
		if err := rows.Scan(&item.Namespace, &item.Key, &item.Value, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan store item: %w", err)
		}
		items = append(items, item)
	}
	return items, nil
}

// ListNamespaces returns distinct namespaces matching optional prefix/suffix and depth constraints.
func (r *StoreRepository) ListNamespaces(ctx context.Context, prefix, suffix []string, maxDepth, limit, offset int) ([][]string, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT DISTINCT namespace
		FROM store_items
		WHERE (expires_at IS NULL OR expires_at > NOW())`

	args := []interface{}{}
	argIdx := 1

	if len(prefix) > 0 {
		query += fmt.Sprintf(" AND namespace[1:$%d] = $%d", argIdx, argIdx+1)
		args = append(args, len(prefix), prefix)
		argIdx += 2
	}

	if len(suffix) > 0 {
		query += fmt.Sprintf(" AND namespace[array_length(namespace,1)-$%d+1:] = $%d", argIdx, argIdx+1)
		args = append(args, len(suffix), suffix)
		argIdx += 2
	}

	if maxDepth > 0 {
		query += fmt.Sprintf(" AND array_length(namespace, 1) <= $%d", argIdx)
		args = append(args, maxDepth)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY namespace LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := r.readPool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}
	defer rows.Close()

	var namespaces [][]string
	for rows.Next() {
		var ns []string
		if err := rows.Scan(&ns); err != nil {
			return nil, fmt.Errorf("failed to scan namespace: %w", err)
		}
		namespaces = append(namespaces, ns)
	}
	return namespaces, nil
}
