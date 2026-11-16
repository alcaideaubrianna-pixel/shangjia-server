package cache

import "context"

// SetSample reads at most limit distinct members from the specified set.
// It never enumerates all sets or removes sampled members. Nonpositive limits
// do no work; the hard cap bounds transport and allocation for all callers.
func SetSample(ctx context.Context, key string, limit int) ([]string, error) {
	if key == "" || limit <= 0 {
		return nil, nil
	}
	if limit > 256 {
		limit = 256
	}
	client, err := RedisClient()
	if err != nil {
		return nil, err
	}
	value, err := client.Do(ctx, "SRANDMEMBER", key, limit)
	if err != nil || value == nil || value.IsNil() {
		return nil, err
	}
	return value.Strings(), nil
}

// SetRemove removes one known member from the specified set.
func SetRemove(ctx context.Context, key, member string) error {
	if key == "" || member == "" {
		return nil
	}
	client, err := RedisClient()
	if err != nil {
		return err
	}
	_, err = client.Do(ctx, "SREM", key, member)
	return err
}
