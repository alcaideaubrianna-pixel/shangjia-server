package runtime

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

const (
	accountMediaPartSize             = 1 << 20
	accountMediaPoolSize       int64 = 8
	accountMediaDefaultThreads       = 6
	accountMediaMaxThreads           = 8
)

func normalizeAccountMediaThreads(threads int) int {
	if threads < 1 {
		return 1
	}
	if threads > accountMediaMaxThreads {
		return accountMediaMaxThreads
	}
	return threads
}

type accountMediaTransferManager struct {
	mu    sync.Mutex
	pools map[int64]*accountMediaTransferPool
}

type accountMediaTransferPool struct {
	mu          sync.Mutex
	base        *telegram.Client
	connections map[int]*accountMediaTransferConnection
}

type accountMediaTransferConnection struct {
	client  *tg.Client
	close   func() error
	refs    int
	retired bool
}

var globalAccountMediaTransfer = &accountMediaTransferManager{pools: make(map[int64]*accountMediaTransferPool)}

func (m *accountMediaTransferManager) download(ctx context.Context, accountID int64, base *telegram.Client, location tg.InputFileLocationClass, path string, fileSize int64, dc, maxThreads int) error {
	if base == nil {
		return errors.New("Telegram媒体传输客户端为空")
	}
	if maxThreads < 1 {
		maxThreads = accountMediaMaxThreads
	}
	if maxThreads > accountMediaMaxThreads {
		maxThreads = accountMediaMaxThreads
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
		threads := accountMediaBestThreads(fileSize, maxThreads)
		func() {
			defer release()
			_, err = downloader.NewDownloader().WithPartSize(accountMediaPartSize).Download(client, location).WithThreads(threads).ToPath(ctx, path)
		}()
		if err == nil {
			return nil
		}
		if targetDC, ok := accountMediaFileMigrate(err); ok && targetDC != dc {
			lastErr = err
			m.invalidate(accountID, base, dc)
			dc = targetDC
			continue
		}
		if !accountMediaConnectionInvalid(err) {
			return err
		}
		lastErr = err
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

func (m *accountMediaTransferManager) acquire(ctx context.Context, accountID int64, base *telegram.Client, dc int) (*tg.Client, func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	pool := m.pools[accountID]
	if pool == nil {
		pool = &accountMediaTransferPool{base: base, connections: make(map[int]*accountMediaTransferConnection)}
		m.pools[accountID] = pool
	} else if pool.base != base {
		pool.closeLocked()
		pool.base = base
		pool.connections = make(map[int]*accountMediaTransferConnection)
	}
	return pool.acquire(ctx, dc)
}

func (p *accountMediaTransferPool) acquire(ctx context.Context, dc int) (*tg.Client, func(), error) {
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
		invoker, err = p.base.Pool(accountMediaPoolSize)
	} else {
		invoker, err = p.base.DC(ctx, dc, accountMediaPoolSize)
	}
	if err != nil {
		return nil, nil, err
	}
	connection := &accountMediaTransferConnection{client: tg.NewClient(invoker), close: invoker.Close, refs: 1}
	p.connections[dc] = connection
	return connection.client, func() { p.release(connection) }, nil
}

func (p *accountMediaTransferPool) release(connection *accountMediaTransferConnection) {
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

func (m *accountMediaTransferManager) invalidate(accountID int64, base *telegram.Client, dc int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	pool := m.pools[accountID]
	if pool == nil || pool.base != base {
		return
	}
	pool.mu.Lock()
	defer pool.mu.Unlock()
	connection := pool.connections[dc]
	if connection == nil {
		return
	}
	connection.retired = true
	delete(pool.connections, dc)
	if connection.refs == 0 {
		_ = connection.close()
	}
}

func (p *accountMediaTransferPool) closeLocked() {
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

func accountMediaConnectionInvalid(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "auth_bytes_invalid") || strings.Contains(message, "dc is closed")
}

func accountMediaFileMigrate(err error) (int, bool) {
	rpcErr, ok := tgerr.As(err)
	if !ok || !rpcErr.IsOneOf("FILE_MIGRATE") || rpcErr.Argument <= 0 {
		return 0, false
	}
	return rpcErr.Argument, true
}

func accountMediaBestThreads(size int64, maxThreads int) int {
	threads := 1
	switch {
	case size > 50<<20:
		threads = 8
	case size > 10<<20:
		threads = 4
	case size > 2<<20:
		threads = 2
	}
	if threads > maxThreads {
		threads = maxThreads
	}
	if threads < 1 {
		return 1
	}
	return threads
}
