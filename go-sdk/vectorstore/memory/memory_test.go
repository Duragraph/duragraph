package memory_test

import (
	"context"
	"testing"

	"github.com/duragraph/duragraph/go-sdk/vectorstore"
	"github.com/duragraph/duragraph/go-sdk/vectorstore/memory"
)

func TestAddAndSearch(t *testing.T) {
	ctx := context.Background()
	store := memory.New()

	docs := []vectorstore.Document{
		{ID: "1", Content: "hello world", Embedding: []float64{1, 0, 0}},
		{ID: "2", Content: "goodbye world", Embedding: []float64{0, 1, 0}},
		{ID: "3", Content: "hello again", Embedding: []float64{0.9, 0.1, 0}},
	}
	if err := store.AddDocuments(ctx, docs); err != nil {
		t.Fatalf("AddDocuments: %v", err)
	}

	results, err := store.SimilaritySearch(ctx, []float64{1, 0, 0}, 2)
	if err != nil {
		t.Fatalf("SimilaritySearch: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Document.ID != "1" {
		t.Errorf("top result ID = %q, want '1'", results[0].Document.ID)
	}
	if results[0].Score < 0.99 {
		t.Errorf("top score = %f, want ~1.0", results[0].Score)
	}
	if results[1].Document.ID != "3" {
		t.Errorf("second result ID = %q, want '3'", results[1].Document.ID)
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	store := memory.New()

	docs := []vectorstore.Document{
		{ID: "1", Content: "a", Embedding: []float64{1, 0}},
		{ID: "2", Content: "b", Embedding: []float64{0, 1}},
	}
	_ = store.AddDocuments(ctx, docs)
	_ = store.Delete(ctx, []string{"1"})

	results, _ := store.SimilaritySearch(ctx, []float64{1, 0}, 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result after delete, got %d", len(results))
	}
	if results[0].Document.ID != "2" {
		t.Errorf("remaining doc ID = %q, want '2'", results[0].Document.ID)
	}
}

func TestEmptySearch(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	results, err := store.SimilaritySearch(ctx, []float64{1, 0}, 5)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearchKLargerThanDocs(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	_ = store.AddDocuments(ctx, []vectorstore.Document{
		{ID: "1", Content: "only", Embedding: []float64{1}},
	})
	results, _ := store.SimilaritySearch(ctx, []float64{1}, 100)
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}
