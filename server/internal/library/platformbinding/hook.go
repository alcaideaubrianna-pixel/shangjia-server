package platformbinding

import (
	"context"
	"sync"
)

type ApprovedEvent struct {
	AppID     string
	AppName   string
	TenantID  int64
	BindingID int64
}

type ApprovedHandler func(context.Context, ApprovedEvent) error

var (
	handlersMu sync.RWMutex
	handlers   []ApprovedHandler
)

func RegisterApprovedHandler(handler ApprovedHandler) {
	if handler == nil {
		return
	}
	handlersMu.Lock()
	defer handlersMu.Unlock()
	handlers = append(handlers, handler)
}

func EmitApproved(ctx context.Context, event ApprovedEvent) []error {
	handlersMu.RLock()
	registered := append([]ApprovedHandler(nil), handlers...)
	handlersMu.RUnlock()
	errors := make([]error, 0)
	for _, handler := range registered {
		if err := handler(ctx, event); err != nil {
			errors = append(errors, err)
		}
	}
	return errors
}
