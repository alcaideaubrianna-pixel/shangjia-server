package telegrammedia

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/utility/sensitive"
)

// DownloadURL buffers a bounded asset. Large file callers should use DownloadFile.
func DownloadURL(ctx context.Context, client *http.Client, source string, maxBytes int64) ([]byte, error) {
	if maxBytes == 0 {
		maxBytes = DefaultMaxBytes
	}
	var data bytes.Buffer
	err := transfer(ctx, client, source, maxBytes, 3, func() (io.Writer, func(bool) error, error) {
		data.Reset()
		return &data, func(bool) error { return nil }, nil
	})
	if err != nil {
		return nil, err
	}
	return data.Bytes(), nil
}

// TransferOptions lets callers that already schedule retries avoid nested retries.
type TransferOptions struct{ Attempts int }

type HTTPStatusError struct{ StatusCode int }

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("媒体下载返回状态码:%d", e.StatusCode)
}

// DownloadFile streams to a private temporary file and atomically renames it.
// Interrupted transfers are removed before retrying; no partial file is published.
func DownloadFile(ctx context.Context, client *http.Client, source, destination string, options ...TransferOptions) error {
	if strings.TrimSpace(destination) == "" {
		return gerror.New("媒体下载目标路径为空")
	}
	attempts := 3
	if len(options) > 0 && options[0].Attempts > 0 {
		attempts = options[0].Attempts
	}
	return transfer(ctx, client, source, 0, attempts, func() (io.Writer, func(bool) error, error) {
		if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			return nil, nil, err
		}
		file, err := os.CreateTemp(filepath.Dir(destination), ".telegram-*.part")
		if err != nil {
			return nil, nil, err
		}
		finish := func(success bool) error {
			defer os.Remove(file.Name())
			closeErr := file.Close()
			if !success || closeErr != nil {
				return closeErr
			}
			return os.Rename(file.Name(), destination)
		}
		return file, finish, nil
	})
}

func transfer(ctx context.Context, client *http.Client, source string, maxBytes int64, attempts int, open func() (io.Writer, func(bool) error, error)) error {
	if strings.TrimSpace(source) == "" {
		return gerror.New("Telegram媒体下载地址为空")
	}
	if maxBytes < 0 {
		return gerror.New("媒体下载大小限制无效")
	}
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		retry, err := transferOnce(ctx, client, source, maxBytes, open)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retry || attempt == attempts || ctx.Err() != nil {
			break
		}
		g.Log().Warning(ctx, "Telegram媒体下载临时失败，准备重试", g.Map{"stage": "download_file", "attempt": attempt, "err": sensitive.Diagnostic(err.Error())})
		if err = waitRetry(ctx, attempt, lastErr); err != nil {
			lastErr = err
			break
		}
	}
	if errors.Is(lastErr, context.Canceled) {
		return gerror.Wrap(context.Canceled, "Telegram媒体下载已取消")
	}
	if errors.Is(lastErr, context.DeadlineExceeded) {
		return gerror.Wrap(context.DeadlineExceeded, "Telegram媒体下载超时")
	}
	var status *HTTPStatusError
	if errors.As(lastErr, &status) {
		return status
	}
	return gerror.Newf("下载Telegram媒体失败：%s", sensitive.Diagnostic(lastErr.Error()))
}

func transferOnce(ctx context.Context, client *http.Client, source string, maxBytes int64, open func() (io.Writer, func(bool) error, error)) (bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return false, err
	}
	started := time.Now()
	var mu sync.Mutex
	fields := g.Map{"event": "media_http_download", "method": "GET"}
	if parsed, parseErr := url.Parse(source); parseErr == nil {
		fields["destinationHost"] = parsed.Hostname()
	}
	set := func(key string, value interface{}) { mu.Lock(); fields[key] = value; mu.Unlock() }
	mark := func(key string) { set(key, time.Since(started).Milliseconds()) }
	trace := &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) { mark("dnsStartMs") },
		DNSDone: func(info httptrace.DNSDoneInfo) {
			mark("dnsDoneMs")
			if info.Err != nil {
				set("dnsError", sensitive.Diagnostic(info.Err.Error()))
			}
		},
		ConnectStart: func(_, _ string) { mark("connectStartMs") },
		ConnectDone: func(_, _ string, e error) {
			mark("connectDoneMs")
			if e != nil {
				set("connectError", sensitive.Diagnostic(e.Error()))
			}
		},
		TLSHandshakeStart: func() { mark("tlsStartMs") },
		TLSHandshakeDone: func(_ tls.ConnectionState, e error) {
			mark("tlsDoneMs")
			if e != nil {
				set("tlsError", sensitive.Diagnostic(e.Error()))
			}
		},
		GotConn: func(info httptrace.GotConnInfo) { mark("gotConnectionMs"); set("connectionReused", info.Reused) },
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			mark("requestWrittenMs")
			if info.Err != nil {
				set("writeError", sensitive.Diagnostic(info.Err.Error()))
			}
		},
		GotFirstResponseByte: func() { mark("firstByteMs") },
	}
	request = request.WithContext(httptrace.WithClientTrace(ctx, trace))
	defer func() {
		hostname, _ := os.Hostname()
		mu.Lock()
		defer mu.Unlock()
		fields["instance"] = hostname
		fields["durationMs"] = time.Since(started).Milliseconds()
		if ctx.Err() != nil {
			fields["contextError"] = ctx.Err().Error()
		}
		g.Log().Info(ctx, "媒体HTTP下载诊断", fields)
	}()
	request.Header.Set("User-Agent", "youban-server/media-cache")
	response, err := client.Do(request)
	if err != nil {
		set("error", sensitive.Diagnostic(err.Error()))
		return Retryable(err), err
	}
	set("httpStatus", response.StatusCode)
	set("contentLength", response.ContentLength)
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response.StatusCode == 408 || response.StatusCode == 429 || response.StatusCode >= 500, &HTTPStatusError{StatusCode: response.StatusCode}
	}
	if maxBytes > 0 && response.ContentLength > maxBytes {
		return false, gerror.New("Telegram媒体超过下载大小限制")
	}
	writer, finish, err := open()
	if err != nil {
		return false, err
	}
	var reader io.Reader = response.Body
	if maxBytes > 0 {
		reader = io.LimitReader(reader, maxBytes+1)
	}
	count, err := io.Copy(writer, reader)
	set("transferredBytes", count)
	if err != nil {
		set("error", sensitive.Diagnostic(err.Error()))
	}
	if err != nil {
		_ = finish(false)
		return Retryable(err), err
	}
	if count == 0 || maxBytes > 0 && count > maxBytes {
		_ = finish(false)
		return false, gerror.New("Telegram媒体为空或超过下载大小限制")
	}
	return false, finish(true)
}
