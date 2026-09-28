package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

type sendOptions struct {
	session         string
	project         string
	message         string
	steer           bool
	clientMessageID string
	recoverOnly     bool
}

// sendAPIRequest mirrors the daemon's SendSessionMessageRequest body for
// POST /api/v1/sessions/{id}/send. The CLI keeps its own copy so it need not
// import httpd.
type sendAPIRequest struct {
	Message string `json:"message"`
}

// The steering DTOs mirror the daemon conversation API without coupling the
// thin CLI to the HTTP controller package.
type conversationMessageAPIRequest struct {
	Text            string `json:"text"`
	ClientMessageID string `json:"clientMessageId"`
	RecoverOnly     bool   `json:"recoverOnly,omitempty"`
}

type steerOrSendAPIResponse struct {
	Outcome        string `json:"outcome"`
	TurnID         string `json:"turnId"`
	ProviderTurnID string `json:"providerTurnId"`
	ActivityID     string `json:"activityId"`
	State          string `json:"state"`
	Duplicate      bool   `json:"duplicate"`
}

func newSendCommand(ctx *commandContext) *cobra.Command {
	var opts sendOptions
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send a message to a running agent session",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return ctx.sendMessage(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.session, "session", "", "Session id or agent number (required)")
	addSessionProjectFlag(cmd.Flags(), &opts.project, "Project id that scopes a bare agent number")
	cmd.Flags().StringVar(&opts.message, "message", "", "Message body (required unless --recover-only)")
	cmd.Flags().BoolVar(&opts.steer, "steer", false, "Steer an active Chat turn, or start the next turn when idle")
	cmd.Flags().StringVar(&opts.clientMessageID, "client-message-id", "", "Stable steering delivery handle")
	cmd.Flags().BoolVar(&opts.recoverOnly, "recover-only", false, "Recover a steering result without contacting the provider")
	return cmd
}

func (c *commandContext) sendMessage(ctx context.Context, opts sendOptions) error {
	if opts.recoverOnly && !opts.steer {
		return usageError{errors.New("usage: --recover-only requires --steer")}
	}
	if strings.TrimSpace(opts.clientMessageID) != "" && !opts.steer {
		return usageError{errors.New("usage: --client-message-id requires --steer")}
	}
	if opts.recoverOnly && strings.TrimSpace(opts.clientMessageID) == "" {
		return usageError{errors.New("usage: --recover-only requires --client-message-id")}
	}
	if !opts.recoverOnly && strings.TrimSpace(opts.message) == "" {
		return usageError{errors.New("usage: --message is required")}
	}
	message := opts.message
	session := strings.TrimSpace(opts.session)
	if session == "" {
		return usageError{errors.New("usage: --session is required")}
	}
	sender := strings.TrimSpace(os.Getenv("OPEN_AGENTS_SESSION_ID"))

	// The sender's own number also scopes a bare target, but only on a path that
	// already has to ask the daemon: --recover-only is meant to be a pure local
	// lookup and must not gain a dependency it did not have.
	refOpts := resolveSessionRefOptions{Project: opts.project, FromSessionID: sender}
	if opts.recoverOnly {
		refOpts.FromSessionID = ""
	}
	target, err := c.resolveSendTarget(ctx, session, refOpts)
	if err != nil {
		return err
	}

	// Best effort: a failed lookup only costs the nicer prefix. Never let it cost
	// the send, and never make it for a --recover-only send, which is meant to
	// stay a pure local lookup.
	if !opts.recoverOnly && sender != "" {
		message = agentNumPrefix(sender, c.bestEffortSenderNum(ctx, sender).Num) + message
	}

	// PathEscape: session ids are already "-"/digit safe, but may later come
	// from sanitized issue refs; keep the URL well-formed regardless.
	path := "sessions/" + url.PathEscape(target.SessionID)
	if !opts.steer {
		return c.postJSON(ctx, path+"/send", sendAPIRequest{Message: message}, nil)
	}
	return c.steerMessage(ctx, path, message, strings.TrimSpace(opts.clientMessageID), opts.recoverOnly)
}

// bestEffortSenderNum asks the daemon which number the sending session owns, so
// the recipient sees "[from 6]" rather than an id they cannot type back.
//
// It goes through the resolver rather than the session read because the number
// is the resolver's answer; the session body need not carry one. Any failure
// yields a zero number, which keeps the full id prefix. The sender is resolved
// unscoped: the sender's own project is the one thing this lookup cannot know
// before it makes the call, and an id match is exact either way.
func (c *commandContext) bestEffortSenderNum(ctx context.Context, sender string) resolvedSessionRef {
	if sender == "" {
		return resolvedSessionRef{}
	}
	// The digits-only shortcut in resolveSessionRef does not apply here: the
	// sender's ref is an id, and asking is the entire point of this call.
	ref, err := c.requestSessionRef(ctx, sender, "")
	if err != nil {
		return resolvedSessionRef{}
	}
	return ref
}

func (c *commandContext) steerMessage(
	ctx context.Context,
	sessionPath, message, clientMessageID string,
	recoverOnly bool,
) error {
	if clientMessageID == "" {
		clientMessageID = uuid.NewString()
	}
	var result steerOrSendAPIResponse
	err := c.postJSON(ctx, sessionPath+"/conversation/steer-or-send", conversationMessageAPIRequest{
		Text: message, ClientMessageID: clientMessageID, RecoverOnly: recoverOnly,
	}, &result)
	if err != nil {
		var responseErr apiResponseError
		hasResponseErr := errors.As(err, &responseErr)
		if hasResponseErr && responseErr.ErrorBody.Code == "CHAT_STEER_UNCERTAIN" {
			if recoverOnly {
				return fmt.Errorf("%w; delivery handle %s remains unresolved and was not resent", err, clientMessageID)
			}
			return fmt.Errorf(
				"%w; recover this delivery without resending it: open-agents send --session %s --steer --recover-only --client-message-id %s",
				err, strings.TrimPrefix(sessionPath, "sessions/"), clientMessageID)
		}
		if errors.Is(err, errDaemonUnavailable) || !hasResponseErr || responseErr.StatusCode >= 500 {
			return fmt.Errorf(
				"%w; outcome is unknown for delivery handle %s; retry safely with --client-message-id %s",
				err, clientMessageID, clientMessageID)
		}
		return err
	}

	if result.Outcome == "steered" {
		if recoverOnly {
			_, _ = fmt.Fprintf(c.deps.Out,
				"Recovered steering receipt for turn %s with delivery handle %s; agent action is not confirmed.\n",
				result.ProviderTurnID, clientMessageID)
			return nil
		}
		_, _ = fmt.Fprintf(c.deps.Out,
			"Steering accepted by provider for active turn %s with delivery handle %s; agent action is not confirmed.\n",
			result.ProviderTurnID, clientMessageID)
		return nil
	}
	if result.Duplicate {
		_, _ = fmt.Fprintln(c.deps.Out, "Message was already accepted; delivery state is unchanged.")
		return nil
	}
	_, _ = fmt.Fprintf(c.deps.Out,
		"No active turn to steer. Message accepted as a normal Chat turn in %s state with delivery handle %s; provider delivery is not confirmed.\n",
		result.State, clientMessageID)
	return nil
}
