// Package pgvector provides a PostgreSQL + pgvector vector store implementation.
//
// Requires a PostgreSQL database with the pgvector extension installed.
// Uses database/sql for connectivity — bring your own driver (e.g., lib/pq or pgx).
//
// # Usage
//
//	db, _ := sql.Open("postgres", connStr)
//	store, _ := pgvector.New(db, pgvector.WithTable("embeddings"))
//	store.AddDocuments(ctx, docs)
//	results, _ := store.SimilaritySearch(ctx, queryVec, 5)
package pgvector

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/duragraph/duragraph-go/vectorstore"
)

const defaultTable = "vector_documents"

// StoreOption configures the pgvector store.
type StoreOption func(*Store)

// WithTable sets the table name.
func WithTable(table string) StoreOption {
	return func(s *Store) { s.table = table }
}

// Store is a PostgreSQL vector store using pgvector.
type Store struct {
	db    *sql.DB
	table string
}

// New creates a new pgvector store. Call EnsureTable to create the
// table if it does not already exist.
func New(db *sql.DB, opts ...StoreOption) *Store {
	s := &Store{db: db, table: defaultTable}
	for _, o := range opts {
		o(s)
	}
	return s
}

// EnsureTable creates the documents table and pgvector extension if they
// do not exist.
func (s *Store) EnsureTable(ctx context.Context, dimensions int) error {
	queries := []string{
		"CREATE EXTENSION IF NOT EXISTS vector",
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			embedding vector(%d),
			metadata JSONB DEFAULT '{}'
		)`, s.table, dimensions),
	}
	for _, q := range queries {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("pgvector: ensure table: %w", err)
		}
	}
	return nil
}

// AddDocuments inserts or upserts documents into the store.
func (s *Store) AddDocuments(ctx context.Context, docs []vectorstore.Document) error {
	if len(docs) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("pgvector: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	query := fmt.Sprintf( //nolint:gosec
		`INSERT INTO %s (id, content, embedding, metadata) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (id) DO UPDATE SET content=$2, embedding=$3, metadata=$4`,
		s.table,
	)
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("pgvector: prepare: %w", err)
	}
	defer stmt.Close()

	for _, d := range docs {
		metaJSON, _ := json.Marshal(d.Metadata)
		embStr := vectorToString(d.Embedding)
		if _, err := stmt.ExecContext(ctx, d.ID, d.Content, embStr, string(metaJSON)); err != nil {
			return fmt.Errorf("pgvector: insert %s: %w", d.ID, err)
		}
	}

	return tx.Commit()
}

// SimilaritySearch finds the k nearest documents using cosine distance.
func (s *Store) SimilaritySearch(ctx context.Context, query []float64, k int) ([]vectorstore.SearchResult, error) {
	embStr := vectorToString(query)
	q := fmt.Sprintf( //nolint:gosec
		`SELECT id, content, metadata, 1 - (embedding <=> $1::vector) AS score
		 FROM %s ORDER BY embedding <=> $1::vector LIMIT $2`,
		s.table,
	)

	rows, err := s.db.QueryContext(ctx, q, embStr, k)
	if err != nil {
		return nil, fmt.Errorf("pgvector: search: %w", err)
	}
	defer rows.Close()

	var results []vectorstore.SearchResult
	for rows.Next() {
		var (
			id       string
			content  string
			metaJSON string
			score    float64
		)
		if err := rows.Scan(&id, &content, &metaJSON, &score); err != nil {
			return nil, fmt.Errorf("pgvector: scan: %w", err)
		}
		var meta map[string]any
		_ = json.Unmarshal([]byte(metaJSON), &meta)
		results = append(results, vectorstore.SearchResult{
			Document: vectorstore.Document{
				ID:       id,
				Content:  content,
				Metadata: meta,
			},
			Score: score,
		})
	}
	return results, rows.Err()
}

// Delete removes documents by ID.
func (s *Store) Delete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	q := fmt.Sprintf("DELETE FROM %s WHERE id IN (%s)", s.table, strings.Join(placeholders, ",")) //nolint:gosec
	_, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("pgvector: delete: %w", err)
	}
	return nil
}

func vectorToString(v []float64) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = fmt.Sprintf("%g", f)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
