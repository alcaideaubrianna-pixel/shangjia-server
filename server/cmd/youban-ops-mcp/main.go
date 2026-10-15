package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type apiClient struct {
	baseURL string
	token   string
	client  *http.Client
}

type apiEnvelope[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type messageContextInput struct {
	URL       string `json:"url,omitempty" jsonschema:"Telegram message URL"`
	ChatID    string `json:"chatId,omitempty" jsonschema:"Telegram chat ID when URL is unavailable"`
	MessageID int64  `json:"messageId,omitempty" jsonschema:"Telegram message ID when URL is unavailable"`
}

type repairPlanInput struct {
	Action     string   `json:"action" jsonschema:"Controlled operation action"`
	SourceID   int64    `json:"sourceId,omitempty" jsonschema:"Collection source ID"`
	EventIDs   []int64  `json:"eventIds,omitempty" jsonschema:"Explicit collection event IDs"`
	ProfileIDs []int64  `json:"profileIds,omitempty" jsonschema:"Explicit profile IDs"`
	ProfileNos []string `json:"profileNos,omitempty" jsonschema:"Explicit profile numbers"`
	TenantID   int64    `json:"tenantId,omitempty" jsonschema:"Tenant ID required when deleting profiles"`
	AccountID  int64    `json:"accountId,omitempty" jsonschema:"Account ID required when deleting profiles"`
	StartAt    string   `json:"startAt,omitempty" jsonschema:"RFC3339 range start"`
	EndAt      string   `json:"endAt,omitempty" jsonschema:"RFC3339 range end"`
	Limit      int      `json:"limit,omitempty" jsonschema:"Maximum candidates, up to 1000"`
}

type repairApplyInput struct {
	PlanID       string `json:"planId" jsonschema:"Plan ID returned by collect_preview_repair"`
	ConfirmToken string `json:"confirmToken" jsonschema:"Confirmation token returned by collect_preview_repair"`
}

type profileOperationPreviewInput struct {
	Operation  string   `json:"operation" jsonschema:"offline, republish, or delete"`
	ProfileIDs []int64  `json:"profileIds,omitempty" jsonschema:"Profile IDs"`
	ProfileNos []string `json:"profileNos,omitempty" jsonschema:"Profile numbers"`
	TenantID   int64    `json:"tenantId,omitempty" jsonschema:"Tenant ID required for delete"`
	AccountID  int64    `json:"accountId,omitempty" jsonschema:"Account ID required for delete"`
}

type missingVerificationPreviewInput struct {
	SourceID int64  `json:"sourceId,omitempty" jsonschema:"Optional collection source ID"`
	StartAt  string `json:"startAt" jsonschema:"RFC3339 range start, inclusive"`
	EndAt    string `json:"endAt" jsonschema:"RFC3339 range end, exclusive"`
	Limit    int    `json:"limit,omitempty" jsonschema:"Maximum candidates, up to 1000"`
}

type repairStatusInput struct {
	PlanID string `json:"planId" jsonschema:"Operation plan ID"`
}

type profileMediaInput struct {
	ProfileIDs []int64 `json:"profileIds" jsonschema:"One to twenty profile IDs"`
}

type structuredOutput struct {
	Data map[string]any `json:"data"`
}

type resolvedProfileOutput struct {
	Found                  bool   `json:"found"`
	ProfileID              int64  `json:"profileId,omitempty"`
	ProfileNo              string `json:"profileNo,omitempty"`
	IsCollected            bool   `json:"isCollected"`
	CollectSourceID        int64  `json:"collectSourceId,omitempty"`
	CollectSourceTitle     string `json:"collectSourceTitle,omitempty"`
	CollectSourceUsername  string `json:"collectSourceUsername,omitempty"`
	CollectSourceChatID    string `json:"collectSourceChatId,omitempty"`
	CollectSourceMessageID int64  `json:"collectSourceMessageId,omitempty"`
	CollectSourceURL       string `json:"collectSourceUrl,omitempty"`
	CollectEventID         int64  `json:"collectEventId,omitempty"`
	CollectEventStatus     string `json:"collectEventStatus,omitempty"`
	CollectRuleID          int64  `json:"collectRuleId,omitempty"`
	CollectRuleName        string `json:"collectRuleName,omitempty"`
	Diagnosis              string `json:"diagnosis,omitempty"`
}

func main() {
	client := &apiClient{
		baseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("YOUBAN_AI_OPS_BASE_URL")), "/"),
		token:   strings.TrimSpace(os.Getenv("YOUBAN_AI_OPS_TOKEN")),
		client:  &http.Client{Timeout: 30 * time.Second},
	}
	if client.baseURL == "" || client.token == "" {
		log.Fatal("YOUBAN_AI_OPS_BASE_URL and YOUBAN_AI_OPS_TOKEN are required")
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "youban-ops", Version: "v1.0.0"}, nil)
	readOnly, destructive := false, true
	mcp.AddTool(server, &mcp.Tool{
		Name: "collect_diagnose_message", Description: "Diagnose collection and publishing state by Telegram message URL or chat/message ID",
		Annotations: &mcp.ToolAnnotations{Title: "Diagnose Telegram message", ReadOnlyHint: true, IdempotentHint: true},
	}, client.diagnoseMessage)
	mcp.AddTool(server, &mcp.Tool{
		Name: "resolve_profile_by_message", Description: "Resolve a Telegram message to its profile number and collection source by URL or chat/message ID",
		Annotations: &mcp.ToolAnnotations{Title: "Resolve profile by Telegram message", ReadOnlyHint: true, IdempotentHint: true},
	}, client.resolveProfileByMessage)
	mcp.AddTool(server, &mcp.Tool{
		Name: "collect_preview_repair", Description: "Create a bounded dry-run repair plan and confirmation token without changing business data",
		Annotations: &mcp.ToolAnnotations{Title: "Preview collection repair", ReadOnlyHint: true, IdempotentHint: false},
	}, client.previewRepair)
	mcp.AddTool(server, &mcp.Tool{
		Name: "collect_apply_repair", Description: "Apply a previously previewed repair plan exactly once",
		Annotations: &mcp.ToolAnnotations{Title: "Apply collection repair", ReadOnlyHint: readOnly, DestructiveHint: &destructive, IdempotentHint: true},
	}, client.applyRepair)
	mcp.AddTool(server, &mcp.Tool{
		Name: "profile_preview_operation", Description: "Preview a bounded profile offline, republish, or delete operation and return a confirmation token",
		Annotations: &mcp.ToolAnnotations{Title: "Preview profile operation", ReadOnlyHint: true, IdempotentHint: false},
	}, client.previewProfileOperation)
	mcp.AddTool(server, &mcp.Tool{
		Name: "profile_apply_operation", Description: "Apply a previously previewed profile operation exactly once",
		Annotations: &mcp.ToolAnnotations{Title: "Apply profile operation", ReadOnlyHint: readOnly, DestructiveHint: &destructive, IdempotentHint: true},
	}, client.applyRepair)
	mcp.AddTool(server, &mcp.Tool{
		Name: "verification_preview_missing", Description: "Preview collected profiles eligible for complete replacement republish after verification recovery",
		Annotations: &mcp.ToolAnnotations{Title: "Preview missing verification repair", ReadOnlyHint: true, IdempotentHint: false},
	}, client.previewMissingVerification)
	mcp.AddTool(server, &mcp.Tool{
		Name: "verification_apply_repair", Description: "Recover verification media and replace the complete profile publication exactly once",
		Annotations: &mcp.ToolAnnotations{Title: "Repair missing verification", ReadOnlyHint: readOnly, DestructiveHint: &destructive, IdempotentHint: true},
	}, client.applyRepair)
	mcp.AddTool(server, &mcp.Tool{
		Name: "ops_plan_status", Description: "Read the current result of an operations plan",
		Annotations: &mcp.ToolAnnotations{Title: "Get operation status", ReadOnlyHint: true, IdempotentHint: true},
	}, client.repairStatus)
	mcp.AddTool(server, &mcp.Tool{
		Name: "profile_media_preview", Description: "Inspect whether profile media can be rebuilt without changing data",
		Annotations: &mcp.ToolAnnotations{Title: "Preview profile media repair", ReadOnlyHint: true, IdempotentHint: true},
	}, client.previewProfileMedia)
	mcp.AddTool(server, &mcp.Tool{
		Name: "profile_media_apply", Description: "Rebuild recoverable media for explicitly selected profiles",
		Annotations: &mcp.ToolAnnotations{Title: "Repair profile media", ReadOnlyHint: readOnly, DestructiveHint: &destructive, IdempotentHint: true},
	}, client.applyRepair)
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}

func (c *apiClient) diagnoseMessage(ctx context.Context, _ *mcp.CallToolRequest, input messageContextInput) (*mcp.CallToolResult, structuredOutput, error) {
	data, err := c.messageContext(ctx, input)
	return nil, structuredOutput{Data: data}, err
}

func (c *apiClient) resolveProfileByMessage(ctx context.Context, _ *mcp.CallToolRequest, input messageContextInput) (*mcp.CallToolResult, resolvedProfileOutput, error) {
	data, err := c.messageContext(ctx, input)
	if err != nil {
		return nil, resolvedProfileOutput{}, err
	}
	return nil, resolvedProfileOutput{
		Found:                  mapBool(data, "found"),
		ProfileID:              mapInt64(data, "profileId"),
		ProfileNo:              mapString(data, "profileNo"),
		IsCollected:            mapBool(data, "isCollected"),
		CollectSourceID:        mapInt64(data, "collectSourceId"),
		CollectSourceTitle:     mapString(data, "collectSourceTitle"),
		CollectSourceUsername:  mapString(data, "collectSourceUsername"),
		CollectSourceChatID:    mapString(data, "collectSourceChatId"),
		CollectSourceMessageID: mapInt64(data, "collectSourceMessageId"),
		CollectSourceURL:       mapString(data, "collectSourceUrl"),
		CollectEventID:         mapInt64(data, "collectEventId"),
		CollectEventStatus:     mapString(data, "collectEventStatus"),
		CollectRuleID:          mapInt64(data, "collectRuleId"),
		CollectRuleName:        mapString(data, "collectRuleName"),
		Diagnosis:              mapString(data, "diagnosis"),
	}, nil
}

func (c *apiClient) messageContext(ctx context.Context, input messageContextInput) (map[string]any, error) {
	query := url.Values{}
	query.Set("url", input.URL)
	query.Set("chatId", input.ChatID)
	if input.MessageID > 0 {
		query.Set("messageId", fmt.Sprint(input.MessageID))
	}
	return c.call(ctx, http.MethodGet, "/api/youban_publish/ai-ops/telegram/message/context?"+query.Encode(), nil)
}

func (c *apiClient) previewRepair(ctx context.Context, _ *mcp.CallToolRequest, input repairPlanInput) (*mcp.CallToolResult, structuredOutput, error) {
	data, err := c.call(ctx, http.MethodPost, "/api/youban_publish/ai-ops/repair/plan", input)
	return nil, structuredOutput{Data: data}, err
}

func (c *apiClient) applyRepair(ctx context.Context, _ *mcp.CallToolRequest, input repairApplyInput) (*mcp.CallToolResult, structuredOutput, error) {
	data, err := c.call(ctx, http.MethodPost, "/api/youban_publish/ai-ops/repair/apply", input)
	return nil, structuredOutput{Data: data}, err
}

func (c *apiClient) previewProfileOperation(ctx context.Context, _ *mcp.CallToolRequest, input profileOperationPreviewInput) (*mcp.CallToolResult, structuredOutput, error) {
	action := map[string]string{"offline": "offline_profiles", "republish": "republish_profiles", "delete": "delete_profiles"}[strings.ToLower(strings.TrimSpace(input.Operation))]
	if action == "" {
		return nil, structuredOutput{}, fmt.Errorf("operation must be offline, republish, or delete")
	}
	limit := 100
	if action == "republish_profiles" {
		limit = 20
	}
	return c.previewRepair(ctx, nil, repairPlanInput{
		Action: action, ProfileIDs: input.ProfileIDs, ProfileNos: input.ProfileNos,
		TenantID: input.TenantID, AccountID: input.AccountID, Limit: limit,
	})
}

func (c *apiClient) previewMissingVerification(ctx context.Context, _ *mcp.CallToolRequest, input missingVerificationPreviewInput) (*mcp.CallToolResult, structuredOutput, error) {
	return c.previewRepair(ctx, nil, repairPlanInput{
		Action: "repair_missing_verification", SourceID: input.SourceID,
		StartAt: input.StartAt, EndAt: input.EndAt, Limit: input.Limit,
	})
}

func (c *apiClient) repairStatus(ctx context.Context, _ *mcp.CallToolRequest, input repairStatusInput) (*mcp.CallToolResult, structuredOutput, error) {
	query := url.Values{"planId": []string{input.PlanID}}
	data, err := c.call(ctx, http.MethodGet, "/api/youban_publish/ai-ops/repair/status?"+query.Encode(), nil)
	return nil, structuredOutput{Data: data}, err
}

func (c *apiClient) previewProfileMedia(ctx context.Context, _ *mcp.CallToolRequest, input profileMediaInput) (*mcp.CallToolResult, structuredOutput, error) {
	return c.previewRepair(ctx, nil, repairPlanInput{Action: "repair_profile_media", ProfileIDs: input.ProfileIDs, Limit: 20})
}

func (c *apiClient) call(ctx context.Context, method, path string, body any) (map[string]any, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-AI-Ops-Token", c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var envelope apiEnvelope[map[string]any]
	if err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode AI ops response: %w", err)
	}
	if resp.StatusCode >= 300 || envelope.Code != 0 {
		return nil, fmt.Errorf("AI ops request failed: %s", envelope.Message)
	}
	return envelope.Data, nil
}

func mapString(data map[string]any, key string) string {
	value, _ := data[key].(string)
	return value
}

func mapBool(data map[string]any, key string) bool {
	value, _ := data[key].(bool)
	return value
}

func mapInt64(data map[string]any, key string) int64 {
	switch value := data[key].(type) {
	case float64:
		return int64(value)
	case json.Number:
		result, _ := value.Int64()
		return result
	default:
		return 0
	}
}
