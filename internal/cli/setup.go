package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/justin-efficient/enzo/internal/config"
	"github.com/justin-efficient/enzo/internal/ghclient"
	"github.com/justin-efficient/enzo/internal/gitrepo"
)

// Setup writes the repo's .enzo file after checking the token works.
func Setup(ctx context.Context, env Env, args []string) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	token := fs.String("token", "", "token to store (otherwise enzo prompts)")
	host := fs.String("host", "", "GitHub Enterprise API URL (default github.com)")
	force := fs.Bool("force", false, "overwrite an existing .enzo file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	root, slug, err := repoContext(env)
	if err != nil {
		return err
	}

	existing, err := config.Load(root)
	switch {
	case err == nil && !*force:
		return fmt.Errorf("%s already exists; pass --force to replace it", config.Path(root))
	case err != nil && !errors.Is(err, config.ErrNotFound):
		return err
	}

	tok := strings.TrimSpace(*token)
	if tok == "" {
		tok, err = readToken(env, slug.String())
		if err != nil {
			return err
		}
	}
	if tok == "" {
		return ErrCanceled
	}

	h := *host
	if h == "" && existing != nil {
		h = existing.Host
	}

	client, err := env.NewClient(tok, h)
	if err != nil {
		return err
	}
	login, err := client.Viewer(ctx)
	if err != nil {
		return err
	}

	cfg := &config.Config{Token: tok, Host: h}
	if existing != nil {
		cfg.DefaultReviewers = existing.DefaultReviewers
		cfg.Project = existing.Project
	}
	board, others := pickBoard(ctx, client, slug, login, cfg)
	if err := config.Save(root, cfg); err != nil {
		return err
	}

	// The .gitignore is settled before anything is printed, so the report is
	// one block describing a finished state rather than a running commentary.
	// The error below still says the token was saved, which is the part that
	// would otherwise be in doubt.
	added, err := config.EnsureIgnored(root)
	if err != nil {
		return fmt.Errorf("saved the token but could not update .gitignore: %w", err)
	}

	headline(env.Stdout, emojiSetup, "set up enzo for %s", slug)
	rr := []row{
		{"", "authenticated", login},
		{"", "wrote", config.Path(root)},
	}
	if board != nil {
		rr = append(rr, row{"", "board", fmt.Sprintf("%s (#%d), for priorities", board.Title, board.Number)})
		if len(others) > 0 {
			rr = append(rr, row{"", "other boards", others + " — edit \"project\" in " + config.FileName + " to use one"})
		}
	}
	if added {
		rr = append(rr, row{"", "added", config.FileName + " to .gitignore"})
	}
	rows(env.Stdout, rr...)
	return nil
}

// pickBoard settles which board priorities live on and records it in cfg. A
// board already pinned stays, so re-running setup does not move it; otherwise it
// is the first candidate, linked boards before the ones your issues are on.
//
// Priority is a convenience, so a token that cannot read boards, or none being
// found, leaves cfg without a board and setup otherwise unchanged. others lists
// the remaining candidates for the report.
func pickBoard(ctx context.Context, client ghclient.Client, slug gitrepo.Slug, login string, cfg *config.Config) (chosen *ghclient.Board, others string) {
	boards, err := client.CandidateBoards(ctx, slug, login)
	if err != nil || len(boards) == 0 {
		return nil, ""
	}

	pick := 0
	if cfg.Project != nil {
		pick = -1
		for i, b := range boards {
			if b.Owner == cfg.Project.Owner && b.Number == cfg.Project.Number {
				pick = i
				break
			}
		}
		if pick < 0 {
			// Pinned, but not among what was found: it was chosen by hand
			// or the guess has moved. Either way, leave it alone.
			return nil, ""
		}
	}
	chosen = &boards[pick]
	cfg.Project = &config.ProjectRef{Owner: chosen.Owner, Number: chosen.Number}

	var rest []string
	for i, b := range boards {
		if i != pick {
			rest = append(rest, fmt.Sprintf("%s (#%d)", b.Title, b.Number))
		}
	}
	return chosen, strings.Join(rest, ", ")
}

// readToken gets a token interactively, or from stdin when piped.
func readToken(env Env, repo string) (string, error) {
	if env.Interactive && env.AskToken != nil {
		return env.AskToken(repo)
	}
	if env.Stdin == nil {
		return "", errors.New("no token given: pass --token or pipe one on stdin")
	}
	sc := bufio.NewScanner(env.Stdin)
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return "", err
		}
		return "", errors.New("no token given: pass --token or pipe one on stdin")
	}
	return strings.TrimSpace(sc.Text()), nil
}
