package cli

import (
	"context"
	"errors"

	"github.com/justin-efficient/enzo/internal/config"
	"github.com/justin-efficient/enzo/internal/ghclient"
	"github.com/justin-efficient/enzo/internal/gitrepo"
)

// boardRef is the board `enzo setup` pinned in .enzo, or nil when there is
// none or the file cannot be read.
func boardRef(env Env, root string) *config.ProjectRef {
	cfg, err := config.Load(root)
	if err != nil && !errors.Is(err, config.ErrNotFound) {
		return nil
	}
	if cfg == nil {
		return nil
	}
	return cfg.Project
}

// pinnedBoard reads the pinned board's Priority field, or nil when nothing is
// pinned or it cannot be read.
func pinnedBoard(ctx context.Context, client ghclient.Client, ref *config.ProjectRef) *ghclient.PriorityField {
	if ref == nil {
		return nil
	}
	f, err := client.ProjectPriorityField(ctx, ref.Owner, ref.Number)
	if err != nil {
		return nil
	}
	return f
}

// newIssueBoard is the board a new issue is prioritized on: the pinned one,
// else a guess from the issues assigned to you. Nil means no selector.
//
// Priority is a convenience, so a token that cannot read boards, or none
// being found, gets a form without it rather than no form.
func newIssueBoard(ctx context.Context, client ghclient.Client, ref *config.ProjectRef, slug gitrepo.Slug, login string) *ghclient.PriorityField {
	if f := pinnedBoard(ctx, client, ref); f != nil {
		return f
	}
	f, _ := client.BoardPriorityField(ctx, slug, login)
	return f
}

// issueBoard is the board an existing issue is prioritized on, with the
// issue's own item when it is on that board. With a pinned board, an issue
// that is not on it comes back with no ItemID, meaning it will be added on
// save. Without one, an issue on no board gets no selector.
func issueBoard(ctx context.Context, client ghclient.Client, ref *config.ProjectRef, slug gitrepo.Slug, number int) *ghclient.PriorityField {
	if pinned := pinnedBoard(ctx, client, ref); pinned != nil {
		if f, _ := client.IssuePriorityField(ctx, slug, number, pinned.ProjectID); f != nil {
			return f
		}
		return pinned
	}
	f, _ := client.IssuePriorityField(ctx, slug, number, "")
	return f
}
