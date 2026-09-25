package domain

import "testing"

func TestConversationContextResetProviderItemID(t *testing.T) {
	const session = SessionID("manager-2")
	if got := ConversationContextResetProviderItemID(session); got != "open-agents-context-reset:manager-2" {
		t.Fatalf("ConversationContextResetProviderItemID() = %q", got)
	}
}
