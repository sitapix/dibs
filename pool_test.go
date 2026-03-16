package main

import (
	"context"
	"sync/atomic"
	"testing"
)

func TestFanOut_ProcessesAllItems(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	results, cancelled := fanOut(context.Background(), items, 3, func(_ context.Context, n int) int {
		return n * 2
	})
	if cancelled {
		t.Error("expected cancelled=false")
	}
	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}
	sum := 0
	for _, r := range results {
		sum += r
	}
	if sum != 30 {
		t.Errorf("expected sum=30, got %d", sum)
	}
}

func TestFanOut_EmptyItems(t *testing.T) {
	results, cancelled := fanOut(context.Background(), []int{}, 3, func(_ context.Context, n int) int {
		return n
	})
	if cancelled {
		t.Error("expected cancelled=false")
	}
	if results != nil {
		t.Errorf("expected nil results, got %v", results)
	}
}

func TestFanOut_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	items := make([]int, 100)
	for i := range items {
		items[i] = i
	}

	var processed atomic.Int64
	results, cancelled := fanOut(ctx, items, 2, func(_ context.Context, n int) int {
		if processed.Add(1) >= 3 {
			cancel()
		}
		return n
	})

	if !cancelled {
		t.Error("expected cancelled=true")
	}
	if len(results) >= len(items) {
		t.Errorf("expected fewer results than items, got %d/%d", len(results), len(items))
	}
}

func TestFanOut_SingleWorker(t *testing.T) {
	items := []string{"a", "b", "c"}
	results, cancelled := fanOut(context.Background(), items, 1, func(_ context.Context, s string) string {
		return s + "!"
	})
	if cancelled {
		t.Error("expected cancelled=false")
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
}

func TestFanOut_WorkersCappedByItems(t *testing.T) {
	items := []int{1, 2}
	results, cancelled := fanOut(context.Background(), items, 100, func(_ context.Context, n int) int {
		return n * 10
	})
	if cancelled {
		t.Error("expected cancelled=false")
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}
