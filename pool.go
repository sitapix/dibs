package main

import (
	"context"
	"sync"
)

// fanOut runs fn concurrently on each item using at most workers goroutines.
// It returns all results and whether the context was cancelled.
func fanOut[In, Out any](ctx context.Context, items []In, workers int, fn func(context.Context, In) Out) ([]Out, bool) {
	if len(items) == 0 {
		return nil, false
	}
	workers = min(workers, len(items))

	inCh := make(chan In, workers)
	outCh := make(chan Out, workers)

	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for item := range inCh {
				outCh <- fn(ctx, item)
			}
		}()
	}

	go func() {
		defer close(inCh)
		for _, item := range items {
			select {
			case inCh <- item:
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		wg.Wait()
		close(outCh)
	}()

	results := make([]Out, 0, len(items))
	for r := range outCh {
		results = append(results, r)
	}

	return results, ctx.Err() != nil
}
