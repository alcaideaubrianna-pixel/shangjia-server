package sys

import (
	"context"

	"golang.org/x/sync/errgroup"
)

const gotdAlbumUploadConcurrency = 2

func parallelOrderedMap[In, Out any](ctx context.Context, input []In, limit int, mapFn func(context.Context, int, In) (Out, error)) ([]Out, error) {
	if len(input) == 0 {
		return []Out{}, nil
	}
	if limit < 1 {
		limit = 1
	}
	result := make([]Out, len(input))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(limit)
	for index, item := range input {
		index, item := index, item
		group.Go(func() error {
			mapped, err := mapFn(groupCtx, index, item)
			if err != nil {
				return err
			}
			result[index] = mapped
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return result, nil
}
