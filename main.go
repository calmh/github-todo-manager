package main

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"regexp"
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

	exit := 0
	for _, issue := range issues {
		log := slog.With("number", issue.GetNumber(), "title", issue.GetTitle())
		log.Info("Considering issue")

		vars, _ := variablesFromBody(issue.GetBody())
		if v, ok := vars["rrule"]; ok {
			if err := processRecurring(log, v, client, owner, repo, issue); err != nil {
				log.Error("Processing recurring issue", "error", err)
				exit = 1
			}
		}
		if v, ok := vars["due"]; ok {
			if err := processDue(log, v, client, owner, repo, issue); err != nil {
				log.Error("Processing due issue", "error", err)
				exit = 1
			}
		}
	}
	os.Exit(exit)
}

func processDue(log *slog.Logger, when string, client *github.Client, owner string, repo string, issue *github.Issue) error {
	due, err := time.Parse("2006-01-02", when)
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
		log.Info("Cloning recurring issue", "when", when)
		return clone(client, owner, repo, issue)
	} else {
		log.Info("Next recurring occurrence", "time", rr.After(time.Now(), true))
	}
	return nil
}

func clone(cli *github.Client, owner, repo string, iss *github.Issue) error {
	vars, body := variablesFromBody(iss.GetBody())
	title := iss.GetTitle()

	var labels []string
	if v, ok := vars["labels"]; ok {
		labels = strings.Split(v, ",")
		for i, label := range labels {
			labels[i] = strings.TrimSpace(label)
		}
	}

	// Try to parse & execute the title and body as a templates
	body = execTemplate(body)
	title = execTemplate(title)

	_, _, err := cli.Issues.Create(context.Background(), owner, repo, &github.IssueRequest{
		Title:  github.String(title),
		Body:   github.String(body),
		Labels: &labels,
	})
	return err
}

func execTemplate(s string) string {
	fm := template.FuncMap{
		"now": time.Now,
	}
	tpl, err := template.New("body").Funcs(fm).Parse(s)
	if err != nil {
		return s
	}
	buf := new(bytes.Buffer)
	if err := tpl.Execute(buf, nil); err != nil {
		return s
	}
	return buf.String()
}

var variablesSep = regexp.MustCompile(`(?m)^-?--$`)

func variablesFromBody(body string) (map[string]string, string) {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	matches := variablesSep.FindAllStringIndex(body, -1)
	if len(matches) == 0 {
		return nil, body
	}

	markerIdxStart := matches[len(matches)-1][0]
	markerIdxEnd := matches[len(matches)-1][1]

	vars := make(map[string]string)
	for _, line := range strings.Split(body[markerIdxEnd+1:], "\n") {
		before, after, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		vars[strings.ToLower(strings.TrimSpace(before))] = strings.TrimSpace(after)
	}
	return vars, strings.TrimSpace(body[:markerIdxStart]) + "\n"
}
