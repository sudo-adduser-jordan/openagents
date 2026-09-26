package controllers_test

import (
	"context"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	chatsvc "github.com/sudo-adduser-jordan/open-agents/backend/internal/service/chat"
)

// The conversation-service fakes in this package each implement only the methods
// the test that owns them exercises. ClearHistory is declared on the controller
// interface, so every fake needs a stub even though none of these tests call it;
// the no-op bodies below exist so a fake that does exercise it can be promoted
// deliberately rather than silently passing a nil-call test.

func (f *fakeChatService) ClearHistory(context.Context, domain.SessionID) error { return nil }

func (f *fakeConversationService) ClearHistory(context.Context, domain.SessionID) error { return nil }

func (s *editQueuedStub) ClearHistory(context.Context, domain.SessionID) error { return nil }

func (s *cancelQueuedStub) ClearHistory(context.Context, domain.SessionID) error { return nil }

func (s *reorderQueuedStub) ClearHistory(context.Context, domain.SessionID) error { return nil }

func (s *steerStub) ClearHistory(context.Context, domain.SessionID) error { return nil }

func (s *promoteQueuedStub) ClearHistory(context.Context, domain.SessionID) error { return nil }

// DeleteHistoryBefore is declared on the controller interface for the manager
// prefix-trim route. The narrow stubs below need a no-op so they satisfy it;
// the two fakes that exercise history routes define their functional versions
// next to their other history methods.
func (s *editQueuedStub) DeleteHistoryBefore(context.Context, domain.SessionID, string) (chatsvc.DeleteHistoryBeforeResult, error) {
	return chatsvc.DeleteHistoryBeforeResult{}, nil
}

func (s *cancelQueuedStub) DeleteHistoryBefore(context.Context, domain.SessionID, string) (chatsvc.DeleteHistoryBeforeResult, error) {
	return chatsvc.DeleteHistoryBeforeResult{}, nil
}

func (s *reorderQueuedStub) DeleteHistoryBefore(context.Context, domain.SessionID, string) (chatsvc.DeleteHistoryBeforeResult, error) {
	return chatsvc.DeleteHistoryBeforeResult{}, nil
}

func (s *steerStub) DeleteHistoryBefore(context.Context, domain.SessionID, string) (chatsvc.DeleteHistoryBeforeResult, error) {
	return chatsvc.DeleteHistoryBeforeResult{}, nil
}

func (s *promoteQueuedStub) DeleteHistoryBefore(context.Context, domain.SessionID, string) (chatsvc.DeleteHistoryBeforeResult, error) {
	return chatsvc.DeleteHistoryBeforeResult{}, nil
}

func (f *fakeSessionService) Retire(context.Context, domain.SessionID) (bool, error) {
	return false, nil
}
