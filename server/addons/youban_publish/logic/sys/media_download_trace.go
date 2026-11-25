package sys

import (
	"context"
	"crypto/tls"
	"net/http/httptrace"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// Collect timing only, never request URLs, headers or transport credentials.
func mediaFileCacheHTTPTrace(ctx context.Context) (context.Context, func() g.Map) {
	metadata, _ := ctx.Value(mediaFileCacheObservationContextKey{}).(g.Map)
	if metadata["mediaType"] != "video" {
		return ctx, func() g.Map { return nil }
	}
	startedAt := time.Now()
	var mu sync.Mutex
	fields := g.Map{}
	record := func(key string) {
		mu.Lock()
		fields[key] = time.Since(startedAt).Milliseconds()
		mu.Unlock()
	}
	trace := &httptrace.ClientTrace{
		DNSStart:          func(httptrace.DNSStartInfo) { record("dnsStartMs") },
		DNSDone:           func(httptrace.DNSDoneInfo) { record("dnsDoneMs") },
		ConnectStart:      func(string, string) { record("connectStartMs") },
		ConnectDone:       func(string, string, error) { record("connectDoneMs") },
		TLSHandshakeStart: func() { record("tlsStartMs") },
		TLSHandshakeDone:  func(tls.ConnectionState, error) { record("tlsDoneMs") },
		GotConn: func(info httptrace.GotConnInfo) {
			mu.Lock()
			fields["connectionReused"] = info.Reused
			mu.Unlock()
		},
		GotFirstResponseByte: func() { record("firstResponseByteMs") },
	}
	return httptrace.WithClientTrace(ctx, trace), func() g.Map {
		mu.Lock()
		defer mu.Unlock()
		result := g.Map{}
		for key, value := range fields {
			result[key] = value
		}
		return result
	}
}
