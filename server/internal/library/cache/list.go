package cache

import "context"

// ListPushLeft prepends a wake signal without changing its list encoding.
// Consumers retain the existing RPOP ordering and retry semantics.
func ListPushLeft(ctx context.Context, key, value string) error {
	client, err := RedisClient()
	if err != nil {
		return err
	}
	_, err = client.Do(ctx, "LPUSH", key, value)
	return err
}
