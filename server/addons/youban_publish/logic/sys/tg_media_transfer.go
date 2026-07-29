package sys

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

const (
	tgMediaTransferPartSize         = 1 << 20
	tgMediaTransferPoolSize   int64 = 8
	tgMediaTransferMaxThreads       = 8
)

type tgMediaTransferManager struct {
	mu    sync.Mutex
	pools map[int64]*tgMediaTransferAccountPool
}

type tgMediaTransferAccountPool struct {
	mu          sync.Mutex
	base        *telegram.Client
	connections map[int]*tgMediaTransferConnection
}

type tgMediaTransferConnection struct {
	client  *tg.Client
	close   func() error
	refs    int
	retired bool
}

var globalTGMediaTransferManager = &tgMediaTransferManager{
	pools: make(map[int64]*tgMediaTransferAccountPool),
}

func (m *tgMediaTransferManager) download(
	ctx context.Context,
	accountID int64,
	base *telegram.Client,
	location tg.InputFileLocationClass,
	path string,
	fileSize int64,
	dc int,
	maxThreads int,
) error {
	if base == nil {
		return errors.New("TG媒体传输基础客户端为空")
	}
	if maxThreads < 1 {
		maxThreads = tgMediaTransferMaxThreads
	}
	if maxThreads > tgMediaTransferMaxThreads {
		maxThreads = tgMediaTransferMaxThreads
	}
	if dc <= 0 {
		dc = base.Config().ThisDC
	}
	var lastErr error

	for attempt := 0; attempt < 5; attempt++ {
		client, release, err := m.acquire(ctx, accountID, base, dc)
		if err != nil {
			return err
		}
		threads := tgMediaTransferBestThreads(fileSize, maxThreads)
		func() {
			defer release()
			_, err = downloader.NewDownloader().
				WithPartSize(tgMediaTransferPartSize).
				Download(client, location).
				WithThreads(threads).
				ToPath(ctx, path)
		}()
		if err == nil {
			return nil
		}
		if targetDC, ok := tgMediaTransferFileMigrate(err); ok && targetDC != dc {
			lastErr = err
			g.Log().Warningf(ctx, "TG媒体传输收到FILE_MIGRATE，切换DC accountId:%d fromDC:%d toDC:%d attempt:%d", accountID, dc, targetDC, attempt+1)
			m.invalidate(accountID, base, dc)
			dc = targetDC
			continue
		}
		if tgMediaTransferDCClosed(err) {
			lastErr = err
			g.Log().Warningf(ctx, "TG媒体传输DC连接已关闭，重建连接 accountId:%d dc:%d attempt:%d err:%+v", accountID, dc, attempt+1, err)
			m.invalidate(accountID, base, dc)
			if attempt < 4 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Duration(attempt+1) * 500 * time.Millisecond):
				}
			}
			continue
		}
		if !tgMediaTransferAuthInvalid(err) {
			return err
		}
		lastErr = err
		g.Log().Warningf(ctx, "TG媒体传输连接认证失效，重建DC连接 accountId:%d dc:%d attempt:%d err:%+v", accountID, dc, attempt+1, err)
		m.invalidate(accountID, base, dc)
		if attempt < 4 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt+1) * 500 * time.Millisecond):
			}
		}
	}
	return lastErr
}

func (m *tgMediaTransferManager) acquire(ctx context.Context, accountID int64, base *telegram.Client, dc int) (*tg.Client, func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.pools[accountID]
	if p == nil {
		p = &tgMediaTransferAccountPool{
			base:        base,
			connections: make(map[int]*tgMediaTransferConnection),
		}
		m.pools[accountID] = p
	} else if p.base != base {
		p.closeLocked()
		p.base = base
		p.connections = make(map[int]*tgMediaTransferConnection)
	}

	return p.acquire(ctx, dc)
}

func (p *tgMediaTransferAccountPool) acquire(ctx context.Context, dc int) (*tg.Client, func(), error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if connection := p.connections[dc]; connection != nil && !connection.retired {
		connection.refs++
		return connection.client, func() { p.release(connection) }, nil
	}
	var (
		invoker telegram.CloseInvoker
		err     error
	)
	if dc == p.base.Config().ThisDC {
		invoker, err = p.base.Pool(tgMediaTransferPoolSize)
	} else {
		invoker, err = p.base.DC(ctx, dc, tgMediaTransferPoolSize)
	}
	if err != nil {
		return nil, nil, err
	}
	connection := &tgMediaTransferConnection{
		client: tg.NewClient(invoker),
		close:  invoker.Close,
		refs:   1,
	}
	p.connections[dc] = connection
	return connection.client, func() { p.release(connection) }, nil
}

func (p *tgMediaTransferAccountPool) release(connection *tgMediaTransferConnection) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if connection == nil || connection.refs <= 0 {
		return
	}
	connection.refs--
	if connection.retired && connection.refs == 0 {
		_ = connection.close()
	}
}

func (m *tgMediaTransferManager) invalidate(accountID int64, base *telegram.Client, dc int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.pools[accountID]
	if p == nil || p.base != base {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	connection := p.connections[dc]
	if connection == nil {
		return
	}
	connection.retired = true
	delete(p.connections, dc)
	if connection.refs == 0 {
		_ = connection.close()
	}
}

func (p *tgMediaTransferAccountPool) closeLocked() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for dc, connection := range p.connections {
		connection.retired = true
		delete(p.connections, dc)
		if connection.refs == 0 {
			_ = connection.close()
		}
	}
}

func tgMediaTransferAuthInvalid(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "auth_bytes_invalid")
}

func tgMediaTransferDCClosed(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "dc is closed")
}

func tgMediaTransferFileMigrate(err error) (int, bool) {
	rpcErr, ok := tgerr.As(err)
	if !ok || !rpcErr.IsOneOf("FILE_MIGRATE") || rpcErr.Argument <= 0 {
		return 0, false
	}
	return rpcErr.Argument, true
}

func tgMediaTransferBestThreads(size int64, max int) int {
	switch {
	case size > 0 && size < 1<<20:
		return 1
	case size > 0 && size < 5<<20:
		if max < 2 {
			return max
		}
		return 2
	case size > 0 && size < 20<<20:
		if max < 4 {
			return max
		}
		return 4
	default:
		return max
	}
}
