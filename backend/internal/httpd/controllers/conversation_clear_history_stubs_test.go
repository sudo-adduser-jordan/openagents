package controllers_test

import (
	"context"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
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
