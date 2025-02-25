# github-todo-manager

This implements certain automatic actions for GitHub issues that makes it
more usable as a day to day todo app.

## General

Issues can have *variables* attached to them. Variables are expected after
the last horizontal line (`---`) or signature separator (`--`). Each
variable declaration is a single `key: value` line.

Variables can set due dates, recurrence rules, and labels to use when
creating recurring issues.

Issue titles and bodies can include Go template expressions; a predefined
variable `now` has the current time.

## Examples

### Due

Here is an example issue (body) for an issue with a due date. As the date
approaches `github-todo-manager` will add a `due` label and a comment, and
further comments as it becomes due and overdue.

```
Issue body text here
More body text

---
Due: 2023-12-24
```

### Recurring

Issues can be made recurring by adding a [recurrence
rule](https://icalendar.org/iCalendar-RFC-5545/3-8-5-3-recurrence-rule.html).
A "recurring issue" is one that will be cloned when the recurrence rule
fires. When cloning the issue, the variables are not copied. Here's an issue
that will recur on the first Monday of each month, the clone getting a
`todo` label:

```
Issue body text here
More body text

---
RRule: FREQ=MONTHLY;BYMONTHDAY=1,2,3,4,5,6,7;BYDAY=MO
Labels: todo
```

### Advanced

More advanced combinations can be made. The variables for a given issue are
those after the *last* separator, so we can add variables to the template
that will be used for recurring issues. Here's a recurring issue that fires
monthly, and where each clone will have a due date set to a week later.

```
Issue body text here
More body text

---
Due: {{ (now.AddDate 0 0 7).Format "2006-01-02" }}

---
RRule: FREQ=MONTHLY;BYMONTHDAY=1,2,3,4,5,6,7;BYDAY=MO
Labels: todo
```

## Installation

The Docker container can be used as a GitHub action:

```
name: Manage todo items
on:
  schedule:
    - cron: "0 6 * * *"
  workflow_dispatch:

jobs:
  manage-todos:
    name: Manage todo items
    runs-on: ubuntu-latest
    steps:
      - uses: docker://ghcr.io/calmh/github-todo-manager:latest
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}%
```
