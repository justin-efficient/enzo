package cli

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/justin-efficient/enzo/internal/ghclient"
	"github.com/justin-efficient/enzo/internal/gitrepo"
)

// confirmPhrase is what you have to type for `enzo abort` to go ahead. It is
// deliberately not "y": the branch and its unpushed commits do not come back.
const confirmPhrase = "nukefromorbit"

// Abort throws away the attempt on the current branch: the pull request is
// closed, the branch is deleted here and on origin. The issue is left open —
// abandoning an attempt is not the same as abandoning the work, and you may
// well want to `enzo start` it again.
func Abort(ctx context.Context, env Env, args []string) error {
	fs := flag.NewFlagSet("abort", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("`enzo abort` takes no arguments; it acts on the branch you are on (%s)", fs.Arg(0))
	}

	root, slug, err := repoContext(env)
	if err != nil {
		return err
	}
	branch, err := gitrepo.CurrentBranch(root)
	if err != nil {
		return err
	}

	client, login, err := clientFor(ctx, env, root)
	if err != nil {
		return err
	}

	number, ok := parseBranch(login, branch)
	if !ok {
		return fmt.Errorf("%q is not a branch enzo started; abort only destroys branches named %s",
			branch, branchName(login, 0, "")+"-<title>")
	}

	base, err := client.DefaultBranch(ctx, slug)
	if err != nil {
		return err
	}
	if branch == base {
		return fmt.Errorf("refusing to abort %s, the default branch", base)
	}
	pr, err := client.PullRequestForBranch(ctx, slug, branch)
	if err != nil {
		return err
	}

	ok, err = confirmAbort(env, root, slug, branch, number, pr, base)
	if err != nil {
		return err
	}
	if !ok {
		return ErrCanceled
	}

	return destroy(ctx, env, root, client, slug, branch, number, pr, base)
}

// destroy does the irreversible half, stopping at the first failure and
// reporting what it managed. The local switch goes first: it is the step most
// likely to be refused, and being refused there costs nothing.
func destroy(ctx context.Context, env Env, root string, client ghclient.Client, slug gitrepo.Slug, branch string, number int, pr *ghclient.PullRequest, base string) error {
	// The prompt left the cursor on its own line; start the report below it.
	fmt.Fprintln(env.Stdout)

	if err := gitrepo.Switch(root, base); err != nil {
		return fmt.Errorf("nothing was destroyed: %w", err)
	}
	fmt.Fprintf(env.Stdout, "  switched to %s\n", base)

	if pr != nil {
		if err := client.ClosePullRequest(ctx, slug, pr.Number); err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "  closed   PR #%d\n", pr.Number)
		record(env, root, slug.String(), "aborted",
			fmt.Sprintf("PR #%d for #%d on %s", pr.Number, number, branch), pr.URL)
	}

	if gitrepo.RemoteBranchExists(root, "origin", branch) {
		if err := gitrepo.DeleteRemoteBranch(root, "origin", branch); err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "  deleted  origin/%s\n", branch)
	}

	if err := gitrepo.DeleteBranch(root, branch); err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "  deleted  %s\n", branch)
	fmt.Fprintf(env.Stdout, "  left     #%d open\n", number)
	return nil
}

// confirmAbort prints what is about to be destroyed, including anything that
// will not survive it, and waits for the phrase.
func confirmAbort(env Env, root string, slug gitrepo.Slug, branch string, number int, pr *ghclient.PullRequest, base string) (bool, error) {
	fmt.Fprintf(env.Stdout, "about to destroy, in %s:\n", slug)
	fmt.Fprintf(env.Stdout, "  branch %s, here and on origin\n", branch)
	if pr != nil {
		fmt.Fprintf(env.Stdout, "  PR #%d %s\n", pr.Number, pr.Title)
	} else {
		fmt.Fprintln(env.Stdout, "  (no open pull request on it)")
	}

	// Anything that will not come back gets named before the prompt, not after.
	if n, err := gitrepo.Unpushed(root, "origin", branch, base); err == nil && n > 0 {
		fmt.Fprintf(env.Stdout, "  ⚠ %s not on origin — deleting the branch destroys them\n", plural(n, "commit"))
	}
	if dirty, err := gitrepo.DirtyFiles(root); err == nil && len(dirty) > 0 {
		fmt.Fprintf(env.Stdout, "  ⚠ %s with uncommitted changes, which move to %s\n",
			plural(len(dirty), "file"), base)
	}
	fmt.Fprintf(env.Stdout, "\n#%d stays open. A closed PR cannot be deleted, only closed.\n", number)
	fmt.Fprintf(env.Stdout, "type %s to confirm: ", confirmPhrase)

	line, err := readLine(env)
	if err != nil {
		fmt.Fprintln(env.Stdout)
		return false, nil
	}
	if strings.TrimSpace(line) != confirmPhrase {
		fmt.Fprintln(env.Stdout, "not confirmed; nothing was touched")
		return false, nil
	}
	return true, nil
}

// readLine takes one line from stdin. An abort driven from a pipe is fine:
// the phrase is the safety, not the terminal.
func readLine(env Env) (string, error) {
	r := bufio.NewReader(env.Stdin)
	return r.ReadString('\n')
}

// parseBranch reads the issue number back out of a branch enzo named. It is
// the inverse of branchName, and anything that does not match is a branch enzo
// did not create — which abort refuses to touch.
func parseBranch(login, branch string) (int, bool) {
	rest, ok := strings.CutPrefix(branch, login+"/")
	if !ok {
		return 0, false
	}
	digits, _, _ := strings.Cut(rest, "-")
	n, ok := asIssueNumber(digits)
	if !ok || n <= 0 {
		return 0, false
	}
	return n, true
}

// plural renders "1 commit" / "2 commits".
func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
