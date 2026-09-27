package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type managerListOptions struct {
	json bool
}

type managerListOutput struct {
	Data []sessionListEntry `json:"data"`
}

func newManagerCommand(ctx *commandContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manager",
		Short: "Manage manager sessions",
	}
	cmd.AddCommand(newManagerListCommand(ctx))
	cmd.AddCommand(newManagerDoneCommand(ctx))
	return cmd
}

// newManagerDoneCommand backs the re-engagement loop's only exit. A manager
// whose assigned work is finished runs this so the daemon stops nudging it
// instead of waiting out the retry ceiling and raising a human-attention
// notification.
func newManagerDoneCommand(ctx *commandContext) *cobra.Command {
	var sessionID string
	cmd := &cobra.Command{
		Use:   "done",
		Short: "Stop automatic re-engagement for a completed manager",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(sessionID) == "" {
				return usageError{errors.New("--session is required")}
			}
			var res struct {
				OK        bool   `json:"ok"`
				SessionID string `json:"sessionId"`
			}
			if err := ctx.postJSON(cmd.Context(), "managers/"+sessionID+"/done", nil, &res); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Manager %s marked done.\n", res.SessionID)
			return err
		},
	}
	cmd.Flags().StringVar(&sessionID, "session", "", "Manager session id")
	return cmd
}

func newManagerListCommand(ctx *commandContext) *cobra.Command {
	var opts managerListOptions
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List manager sessions",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return ctx.listManagers(cmd.Context(), cmd, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.json, "json", false, "Output as JSON")
	return cmd
}

func (c *commandContext) listManagers(ctx context.Context, cmd *cobra.Command, opts managerListOptions) error {
	var res sessionListResponse
	if err := c.getJSON(ctx, "managers", &res); err != nil {
		return err
	}
	managers := filterAndSortManagers(res.Sessions)
	if opts.json {
		return writeJSON(cmd.OutOrStdout(), managerListOutput{Data: sessionListEntries(managers, nil)})
	}
	return writeManagerList(cmd, managers)
}

func filterAndSortManagers(sessions []sessionDTO) []sessionDTO {
	out := make([]sessionDTO, 0, len(sessions))
	for _, sess := range sessions {
		if sess.Kind != "manager" {
			continue
		}
		out = append(out, sess)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ProjectID != out[j].ProjectID {
			return out[i].ProjectID < out[j].ProjectID
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func writeManagerList(cmd *cobra.Command, sessions []sessionDTO) error {
	out := cmd.OutOrStdout()
	if len(sessions) == 0 {
		_, err := fmt.Fprintln(out, "(no managers)")
		return err
	}
	currentProject := ""
	for _, sess := range sessions {
		if sess.ProjectID != currentProject {
			if currentProject != "" {
				if _, err := fmt.Fprintln(out); err != nil {
					return err
				}
			}
			currentProject = sess.ProjectID
			if _, err := fmt.Fprintf(out, "%s:\n", currentProject); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(out, "  %s", sess.ID); err != nil {
			return err
		}
		parts := managerLineParts(sess)
		if len(parts) > 0 {
			if _, err := fmt.Fprintf(out, "  %s", strings.Join(parts, "  ")); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
	}
	return nil
}

func managerLineParts(sess sessionDTO) []string {
	parts := []string{}
	if !sess.Activity.LastActivityAt.IsZero() {
		parts = append(parts, "("+formatSessionAge(time.Since(sess.Activity.LastActivityAt))+")")
	}
	if sess.Status != "" {
		parts = append(parts, "["+sess.Status+"]")
	}
	if sess.IsTerminated {
		parts = append(parts, "terminated")
	}
	return parts
}
