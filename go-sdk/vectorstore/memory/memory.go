// Package memory provides an in-memory vector store implementation.
//
// Suitable for development, testing, and small datasets. Uses brute-force
// cosine similarity for search.
//
// # Usage
//
//	store := memory.New()
//	store.AddDocuments(ctx, docs)
//	results, err := store.SimilaritySearch(ctx, queryVec, 5)
package memory

import (
	"context"
	"math"
	"sort"
	"sync"

	"github.com/duragraph/duragraph/go-sdk/vectorstore"
)

// Store is an in-memory vector store.
type Store struct {
	mu   sync.RWMutex
	docs map[string]vectorstore.Document
}

// New creates a new in-memory vector store.
func New() *Store {
	return &Store{
		docs: make(map[string]vectorstore.Document),
	}
}

// AddDocuments adds documents to the in-memory store.
func (s *Store) AddDocuments(_ context.Context, docs []vectorstore.Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range docs {
		s.docs[d.ID] = d
	}
	return nil
}

// SimilaritySearch returns the k most similar documents using cosine similarity.
func (s *Store) SimilaritySearch(_ context.Context, query []float64, k int) ([]vectorstore.SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	type scored struct {
		doc   vectorstore.Document
		score float64
	}
	var results []scored

	for _, d := range s.docs {
		if len(d.Embedding) == 0 {
			continue
		}
		score := cosineSimilarity(query, d.Embedding)
		results = append(results, scored{doc: d, score: score})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if k > len(results) {
		k = len(results)
	}

	out := make([]vectorstore.SearchResult, k)
	for i := 0; i < k; i++ {
		out[i] = vectorstore.SearchResult{
			Document: results[i].doc,
			Score:    results[i].score,
		}
	}
	return out, nil
}

// Delete removes documents by ID.
func (s *Store) Delete(_ context.Context, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		delete(s.docs, id)
	}
	return nil
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}
