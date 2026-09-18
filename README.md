# enzo

A tiny simple CLI for GitHub that makes issue lifecycle management easier.

enzo keeps PRs and issues linked.
ezno is stateless.
enzo is pretty.

`enzo [setup]`      # create .enzo file with required tokens etc accessing this repo. do this by default if one does not exist

`enzo [list]`			# list all the open issues assigned to me - selecting one grabs it, top option is "new", esc cancels with nothing to do

`enzo start [sub] [issue-number]` # create a new issue in current repo assigned to me with a new linked draft PR assigned to me no reviewers. sub presents  existining issues that could be parents of this issue

`enzo grab <issue-number>` 	# switch to the origin branch associated with the issue, or link a new origin branch and draft PR and switch to branch

`enzo review` 			# make the current branches PR undrafted. attach default reviewers for that repo.

`enzo finish`			# merge the current branch if the PR is ready. if all reviews are complete, check builds if builds are complete give option to wait for them, merge anyway, or cancel
