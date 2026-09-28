// Package gh creates provider pull requests through the gh CLI: the same tool
// agents use, so creation honors the user's own gh authentication instead of
// a daemon-held token.
package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
)

const defaultGHBinary = "gh"

// Options configures a gh Creator. Binary falls back to gh from PATH.
type Options struct {
	Binary string
}

// Creator shells out to gh for pull request creation and lookup.
type Creator struct {
	binary string
}

var _ ports.PullRequestCreator = (*Creator)(nil)

// New builds a gh Creator.
func New(opts Options) *Creator {
	binary := strings.TrimSpace(opts.Binary)
	if binary == "" {
		binary = defaultGHBinary
	}
	return &Creator{binary: binary}
}

type ghPREntry struct {
	URL    string `json:"url"`
	Number int    `json:"number"`
}

// FindPRByHead returns the open pull request for a head branch, if any.
func (c *Creator) FindPRByHead(ctx context.Context, repoDir, headBranch string) (string, int, bool, error) {
	if err := c.requireBinary(); err != nil {
		return "", 0, false, err
	}
	out, err := c.run(ctx, repoDir, "pr", "list", "--head", headBranch, "--json", "url,number", "--limit", "1")
	if err != nil {
		return "", 0, false, classifyGHError("list pull requests", err)
	}
	var entries []ghPREntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return "", 0, false, fmt.Errorf("gh: parse pr list: %w", err)
	}
	for _, entry := range entries {
		if strings.TrimSpace(entry.URL) == "" {
			continue
		}
		return entry.URL, entry.Number, true, nil
	}
	return "", 0, false, nil
}

// CreatePR creates a pull request from head into base and returns its URL.
func (c *Creator) CreatePR(ctx context.Context, repoDir, base, head, title, body string) (ports.CreatedPullRequest, error) {
	if err := c.requireBinary(); err != nil {
		return ports.CreatedPullRequest{}, err
	}
	args := []string{"pr", "create", "--base", base, "--head", head, "--title", title}
	if strings.TrimSpace(body) != "" {
		args = append(args, "--body", body)
	}
	out, err := c.run(ctx, repoDir, args...)
	if err != nil {
		if isAlreadyExistsError(err) {
			return ports.CreatedPullRequest{}, fmt.Errorf("gh: %w", ports.ErrGHPullRequestExists)
		}
		return ports.CreatedPullRequest{}, classifyGHError("create pull request", err)
	}
	url := strings.TrimSpace(string(out))
	if url == "" {
		return ports.CreatedPullRequest{}, fmt.Errorf("gh: create pull request returned no URL")
	}
	number := parsePRNumber(url)
	return ports.CreatedPullRequest{URL: url, Number: number, Created: true}, nil
}

func (c *Creator) requireBinary() error {
	if _, err := exec.LookPath(c.binary); err != nil {
		return fmt.Errorf("gh: %w", ports.ErrGHNotInstalled)
	}
	return nil
}

func (c *Creator) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.binary, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, &ghError{output: strings.TrimSpace(string(out)), err: err}
	}
	return out, nil
}

// ghError carries gh's stderr alongside the exit error so classifiers can
// match on provider/auth wording without losing the cause.
type ghError struct {
	output string
	err    error
}

func (e *ghError) Error() string {
	if e.output == "" {
		return fmt.Sprintf("gh: %v", e.err)
	}
	return fmt.Sprintf("gh: %s: %v", firstLine(e.output), e.err)
}

func (e *ghError) Unwrap() error { return e.err }

// classifyGHError maps authentication failures to ErrGHAuthMissing and wraps
// everything else with the operation for a 500.
func classifyGHError(op string, err error) error {
	if isAuthError(err) {
		return fmt.Errorf("gh: %s: %w (run `gh auth login`)", op, ports.ErrGHAuthMissing)
	}
	return fmt.Errorf("gh: %s: %w", op, err)
}

func isAuthError(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{
		"not logged into",
		"not authenticated",
		"authentication required",
		"gh auth login",
		"could not resolve to an authentication token",
		"bad credentials",
		"http 401",
		"http 403",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

func isAlreadyExistsError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already exists")
}

// parsePRNumber extracts the trailing pull request number from a canonical
// .../pull/<n> URL. Zero when the URL has no such shape.
func parsePRNumber(url string) int {
	trimmed := strings.TrimRight(strings.TrimSpace(url), "/")
	idx := strings.LastIndex(trimmed, "/pull/")
	if idx < 0 {
		return 0
	}
	var n int
	if _, err := fmt.Sscanf(trimmed[idx+len("/pull/"):], "%d", &n); err != nil {
		return 0
	}
	return n
}

func firstLine(s string) string {
	if line, _, ok := strings.Cut(s, "\n"); ok {
		return strings.TrimSpace(line)
	}
	return strings.TrimSpace(s)
}
