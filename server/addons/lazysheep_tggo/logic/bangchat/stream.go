package bangchat

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	streamTypeMessage = "STREAM_TYPE_MESSAGE"
	streamTypePing    = "STREAM_TYPE_PING"
)

type StreamOption struct {
	MaxMessageUpID string
	MaxChannelUpID string
	IncludeQuote   any
}

type StreamEvent struct {
	Event string
	ID    string
	Data  string
}

type StreamHandler func(ctx context.Context, message json.RawMessage) error

func (c *Client) ListenMessages(ctx context.Context, opt StreamOption, handler StreamHandler) error {
	if c == nil || c.jwt == "" {
		return fmt.Errorf("bangchat client is not registered")
	}
	req, random, err := c.newSignedRequest(ctx, "/v1.Stream/Connect", map[string]any{
		"includeQuote":   opt.IncludeQuote,
		"maxChannelUpId": defaultString(opt.MaxChannelUpID, "0"),
		"maxMessageUpId": defaultString(opt.MaxMessageUpID, "0"),
		"nonce":          "isWeb",
	})
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := streamHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("stream connect failed: status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		return fmt.Errorf("stream connect content-type invalid: %s", ct)
	}
	return readSSE(ctx, resp.Body, func(event StreamEvent) error {
		if strings.TrimSpace(event.Data) == "" {
			return nil
		}
		decoded, err := xorBase64Decode(event.Data, []byte(random))
		if err != nil {
			return nil
		}
		message := streamMessage(decoded)
		if len(message) == 0 || handler == nil {
			return nil
		}
		return handler(ctx, message)
	})
}

func readSSE(ctx context.Context, body io.Reader, emit func(StreamEvent) error) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var event StreamEvent
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := scanner.Text()
		if line == "" {
			if err := emit(event); err != nil {
				return err
			}
			event = StreamEvent{}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			event.Event = value
		case "id":
			event.ID = value
		case "data":
			if event.Data != "" {
				event.Data += "\n"
			}
			event.Data += value
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func streamMessage(decoded []byte) json.RawMessage {
	var payload struct {
		Result struct {
			Type    string          `json:"type"`
			Message json.RawMessage `json:"message"`
		} `json:"result"`
	}
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return nil
	}
	switch payload.Result.Type {
	case streamTypeMessage:
		return payload.Result.Message
	case streamTypePing, "":
		return nil
	default:
		return nil
	}
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
