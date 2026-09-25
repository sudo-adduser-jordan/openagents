package cli

import (
	"context"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

// workflowModeRequest mirrors the daemon's set-workflow-mode request body
// (PATCH /api/v1/sessions/{sessionId}/workflow-mode).
type workflowModeRequest struct {
	WorkflowMode string `json:"workflowMode"`
}

// workflowModeResponse mirrors the daemon's set-workflow-mode response body:
// the summary plus the refreshed session read model.
type workflowModeResponse struct {
	OK           bool       `json:"ok"`
	SessionID    string     `json:"sessionId"`
	WorkflowMode string     `json:"workflowMode"`
	Session      sessionDTO `json:"session"`
}

// newPlanCommand, newManageCommand, and newBuildCommand change a session's
// delivery posture. A workflow-mode command is also one of the kanban review
// lock's release paths: a card frozen in the review column is released so it can
// move with its PR facts again.
func newPlanCommand(ctx *commandContext) *cobra.Command {
	return newWorkflowModeCommand(ctx, "plan", "planning",
		"Move a session into the planning stage",
	)
}

func newManageCommand(ctx *commandContext) *cobra.Command {
	return newWorkflowModeCommand(ctx, "manage", "manager",
		"Move a session into manager mode",
	)
}

func newBuildCommand(ctx *commandContext) *cobra.Command {
	return newWorkflowModeCommand(ctx, "build", "building",
		"Move a session into the building stage",
	)
}

func newWorkflowModeCommand(ctx *commandContext, name, mode, short string) *cobra.Command {
	var opts sessionOptions
	cmd := &cobra.Command{
		Use:   name + " <id>",
		Short: short,
		Long: fmt.Sprintf("Move a session into the %s stage. Setting the delivery "+
			"stage also releases the kanban review lock, so a card frozen in the "+
			"review column may move with its PR facts again.", name),
		Args: oneSessionIDArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := normalizeSessionID(args[0])
			if err != nil {
				return err
			}
			return ctx.setWorkflowMode(cmd.Context(), cmd, id, mode, opts)
		},
	}
	addSessionProjectFlag(cmd.Flags(), &opts.project, "Project id to scope the lookup")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Output as JSON")
	return cmd
}

func (c *commandContext) setWorkflowMode(ctx context.Context, cmd *cobra.Command, id, mode string, opts sessionOptions) error {
	if opts.project != "" {
		if _, err := c.fetchScopedSession(ctx, id, opts.project); err != nil {
			return err
		}
	}
	var res workflowModeResponse
	if err := c.patchJSON(ctx, "sessions/"+url.PathEscape(id)+"/workflow-mode", workflowModeRequest{WorkflowMode: mode}, &res); err != nil {
		return err
	}
	if opts.json {
		return writeJSON(cmd.OutOrStdout(), res)
	}
	sessionID := res.SessionID
	if sessionID == "" {
		sessionID = id
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "session %s set to %s\n", sessionID, res.WorkflowMode)
	return err
}
