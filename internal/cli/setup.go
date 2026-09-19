package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/justin-efficient/enzo/internal/config"
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
	}
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
		{"authenticated", login},
		{"wrote", config.Path(root)},
	}
	if added {
		rr = append(rr, row{"added", config.FileName + " to .gitignore"})
	}
	rows(env.Stdout, rr...)
	return nil
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
