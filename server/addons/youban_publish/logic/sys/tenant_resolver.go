package sys

import (
	"context"

	"hotgo/internal/library/publishtenant"
)

func init() {
	publishtenant.RegisterResolver(func(ctx context.Context) (int64, error) {
		account, err := (&sSysPublish{}).currentAccount(ctx)
		if err != nil {
			return 0, err
		}
		return account.TenantId, nil
	})
}
