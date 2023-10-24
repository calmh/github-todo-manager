package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/google/go-github/v56/github"
	"github.com/teambition/rrule-go"
	"golang.org/x/oauth2"
)

type CLI struct {
	GithubToken         string `required:"" env:"GITHUB_TOKEN"`
	Repository          string `required:"" env:"GITHUB_REPOSITORY"`
	RecurringIssueLabel string `default:"recurring" env:"RECURRING_ISSUE_LABEL"`
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
		State:  "open",
		Labels: []string{cli.RecurringIssueLabel},
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
				rr, err := rrule.StrToRRule(line[6:])
				if err != nil {
					log.Error("Invalid RRULE, skipping", "error", err)
					continue
				}
				rr.DTStart(issue.GetCreatedAt().Time)

				when := rr.Before(time.Now(), true)
				if time.Since(when) <= 24*time.Hour {
					if err := clone(client, owner, repo, issue); err != nil {
						log.Error("Failed to clone issue", "error", err)
					} else {
						log.Info("Cloned issue")
					}
				}
			}
		}
	}
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
