package sys

import "context"

// External steps stay behind the original Telegram implementations. Tests can
// exercise the complete persistence flow without creating real Telegram groups.
type botCreationHooks struct {
	token func(context.Context, string) (string, string, string, error)
	group func(context.Context, *publishAccount, *publishTgAccount, string, string) (*setupGroupResult, error)
	sync  func(context.Context, int64, int64) error
}

func (s *sSysTwoWayBot) creationToken(ctx context.Context, token string) (string, string, string, error) {
	if s.creation != nil && s.creation.token != nil {
		return s.creation.token(ctx, token)
	}
	return s.validateBotToken(ctx, token)
}
func (s *sSysTwoWayBot) creationGroup(ctx context.Context, account *publishAccount, tgAccount *publishTgAccount, existingGroupID, username string) (*setupGroupResult, error) {
	if s.creation != nil && s.creation.group != nil {
		return s.creation.group(ctx, account, tgAccount, existingGroupID, username)
	}
	if existingGroupID != "" {
		return s.setupExistingManagementGroup(ctx, account, tgAccount, existingGroupID, username)
	}
	return s.createManagementGroup(ctx, account, tgAccount, username)
}
func (s *sSysTwoWayBot) creationSync(ctx context.Context, id, tenantID int64) error {
	if s.creation != nil && s.creation.sync != nil {
		return s.creation.sync(ctx, id, tenantID)
	}
	return s.syncRuntimeById(ctx, id, tenantID)
}
