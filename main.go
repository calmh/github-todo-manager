package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/google/go-github/v56/github"
	"github.com/teambition/rrule-go"
	"golang.org/x/oauth2"
)

type CLI struct {
	GithubToken string `required:"" env:"GITHUB_TOKEN"`
	Repository  string `required:"" env:"GITHUB_REPOSITORY"`
}

func main() {
	var cli CLI
	kong.Parse(&cli)

	owner, repo, ok := strings.Cut(cli.Repository, "/")
	if !ok {
		slog.Error("Invalid repository name", "repository", cli.Repository)
		os.Exit(1)
	}

	tc := oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(&oauth2.Token{AccessToken: cli.GithubToken}))
	client := github.NewClient(tc)

	issues, _, err := client.Issues.ListByRepo(context.Background(), owner, repo, &github.IssueListByRepoOptions{
		State: "open",
	})
	if err != nil {
		slog.Error("Listing issues", "error", err)
		os.Exit(1)
	}

	for _, issue := range issues {
		log := slog.With("number", issue.GetNumber(), "title", issue.GetTitle())
		log.Info("Considering issue")

		lines := strings.Split(issue.GetBody(), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "RRULE:") {
				if err := processRecurring(log, line[6:], client, owner, repo, issue); err != nil {
					log.Error("Processing recurring issue", "error", err)
				}
			} else if strings.HasPrefix(line, "Due:") {
				if err := processDue(log, line, client, owner, repo, issue); err != nil {
					log.Error("Processing due issue", "error", err)
				}
			}
		}
	}
}

func processDue(log *slog.Logger, line string, client *github.Client, owner string, repo string, issue *github.Issue) error {
	_, after, _ := strings.Cut(line, ":")
	due, err := time.Parse("2006-01-02", strings.TrimSpace(after))
	if err != nil {
		return fmt.Errorf("invalid date: %w", err)
	}

	log.Info("Processing due issue", "due", due)

	dueInDays := int(time.Until(due).Hours()/24) + 1

	// Set "todo" label if it's due in a week, "due" if it's due in a day.
	var setLabels []string
	if dueInDays <= 7 && !slices.ContainsFunc(issue.Labels, func(label *github.Label) bool {
		return label.GetName() == "todo"
	}) {
		setLabels = append(setLabels, "todo")
	}
	if dueInDays <= 1 && !slices.ContainsFunc(issue.Labels, func(label *github.Label) bool {
		return label.GetName() == "due"
	}) {
		setLabels = append(setLabels, "due")
	}
	if len(setLabels) > 0 {
		_, _, err = client.Issues.AddLabelsToIssue(context.Background(), owner, repo, issue.GetNumber(), setLabels)
		if err != nil {
			return fmt.Errorf("adding labels: %w", err)
		}
	}

	// Comment on issues that are about to become due.
	updated := time.Since(issue.GetUpdatedAt().Time)
	switch {
	case dueInDays <= 0:
		if issue.GetUpdatedAt().Time.Before(due) {
			_, _, err = client.Issues.CreateComment(context.Background(), owner, repo, issue.GetNumber(), &github.IssueComment{
				Body: github.String("This issue is now overdue"),
			})
		}
	case updated >= 30*24*time.Hour && dueInDays <= 7,
		updated >= 7*24*time.Hour && dueInDays <= 2,
		updated >= 24*time.Hour && dueInDays <= 1:
		_, _, err = client.Issues.CreateComment(context.Background(), owner, repo, issue.GetNumber(), &github.IssueComment{
			Body: github.String(fmt.Sprintf("This issue is due in %d days", dueInDays)),
		})
	}
	return err
}

func processRecurring(log *slog.Logger, rule string, client *github.Client, owner string, repo string, issue *github.Issue) error {
	rr, err := rrule.StrToRRule(rule)
	if err != nil {
		return fmt.Errorf("invalid rrule: %w", err)
	}
	rr.DTStart(issue.GetCreatedAt().Time)

	when := rr.Before(time.Now(), true)
	if time.Since(when) <= 24*time.Hour {
		return clone(client, owner, repo, issue)
	} else {
		log.Info("Next recurring occurrence", "time", rr.After(time.Now(), true))
	}
	return nil
}

func clone(cli *github.Client, owner, repo string, iss *github.Issue) error {
	bodyLines := strings.Split(iss.GetBody(), "\n")
	var newBodyLines []string
	beforeHR := true
	var labels []string
	for _, line := range bodyLines {
		if strings.HasPrefix(line, "---") {
			beforeHR = false
		}
		if beforeHR {
			newBodyLines = append(newBodyLines, line)
		}
		if strings.HasPrefix(line, "Labels:") {
			_, after, _ := strings.Cut(line, ":")
			labels = strings.Split(after, ",")
		}
	}
	newBodyLines = append(newBodyLines, "---")
	newBodyLines = append(newBodyLines, fmt.Sprintf("Cloned from #%d", iss.GetNumber()))

	_, _, err := cli.Issues.Create(context.Background(), owner, repo, &github.IssueRequest{
		Title:  github.String(iss.GetTitle()),
		Body:   github.String(strings.Join(newBodyLines, "\n")),
		Labels: &labels,
	})
	return err
}
