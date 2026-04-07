// Package vectorstore provides vector store interfaces and implementations
// for similarity search in AI workflows.
//
// # Usage
//
//	store := memory.New(memory.WithDimensions(1536))
//	store.AddDocuments(ctx, docs)
//	results, err := store.SimilaritySearch(ctx, query, 5)
package vectorstore

import "context"

// Document represents a document with content, embedding, and metadata.
type Document struct {
	// ID uniquely identifies the document.
	ID string `json:"id"`

	// Content is the text content of the document.
	Content string `json:"content"`

	// Embedding is the vector representation. May be nil if not yet embedded.
	Embedding []float64 `json:"embedding,omitempty"`

	// Metadata holds arbitrary key-value pairs.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// SearchResult is a document with a similarity score.
type SearchResult struct {
	Document Document `json:"document"`
	Score    float64  `json:"score"`
}

// Store is the interface for vector stores.
type Store interface {
	// AddDocuments adds documents to the store. Documents should already
	// have their Embedding field populated.
	AddDocuments(ctx context.Context, docs []Document) error

	// SimilaritySearch finds the k most similar documents to the query vector.
	SimilaritySearch(ctx context.Context, query []float64, k int) ([]SearchResult, error)

	// Delete removes documents by their IDs.
	Delete(ctx context.Context, ids []string) error
}

// EmbeddingFunc is a function that computes embeddings for text inputs.
type EmbeddingFunc func(ctx context.Context, texts []string) ([][]float64, error)
