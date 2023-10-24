# github-todo-recurrence

This implements recurring todos (issues) in the GitHub issue tracker. Issues
with the `recurring` label that contain an `RRULE:` line are processed and
cloned according the recurrence rule in question.

When cloning an issue the program stops at the first `---` in the body.
Putting the `RRULE` and `Labels` after such a horisontal line makes them not
show up in the cloned issue.

```
Here's an example issue body. This issue gets cloned to a new issue with the
`todo` label on the first Monday of every month.

---
RRULE:FREQ=MONTHLY;BYMONTHDAY=1,2,3,4,5,6,7;BYDAY=MO
Labels:todo
```

The Docker container can be used as a GitHub action:

```
name: Clone recurring
on:
  schedule:
    - cron: "0 6 * * *"
  workflow_dispatch:

jobs:
  clone-recurring-todos:
    name: Clone recurring todos
    runs-on: ubuntu-latest
    steps:
      - uses: docker://ghcr.io/calmh/github-todo-recurrence:latest
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}%
          # RECURRING_ISSUE_LABEL: recurring
```
