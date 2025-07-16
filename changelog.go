package changelog

import (
	"bytes"
	"regexp"
	"strings"
	"text/template"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
)

func Generate(repoPath string, changelogTemplate string) (string, error) {
	if changelogTemplate == "" {
		changelogTemplate = markdownTemplateDefinition
	}

	commitMessages, err := readCommitMessages(repoPath)
	if err != nil {
		return "", err
	}

	result, err := renderChangelog(commitMessages, changelogTemplate)
	if err != nil {
		return "", err
	}
	return result, nil
}

const markdownTemplateDefinition = `## [unreleased]
{{ range .Sections }}
### {{ .Title }}
{{ range .Commits -}}
- {{ if .Scope }}*{{ .Scope }}* {{ end }}{{ .Description }}
{{ end }}
{{- end -}}`

func readCommitMessages(repoPath string) ([]string, error) {
	r, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, err
	}

	ref, err := r.Head()
	if err != nil {
		return nil, err
	}

	cIter, err := r.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		return nil, err
	}

	var commitMessages []string
	err = cIter.ForEach(func(c *object.Commit) error {
		commitMessages = append(commitMessages, c.Message)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return commitMessages, nil
}

func renderChangelog(commits []string, changelogTemplateString string) (string, error) {
	sections := groupCommitsByTypeTitle(commits)

	changelogTemplate, err := template.New("changelog").Parse(changelogTemplateString)
	if err != nil {
		return "", err
	}

	var renderedTemplate bytes.Buffer
	err = changelogTemplate.Execute(&renderedTemplate, MarkdownTemplateContext{Sections: sections})
	if err != nil {
		return "", err
	}

	return renderedTemplate.String(), nil
}

type CommitTypeTitle string

const (
	CommitTypeTitleFeatures CommitTypeTitle = "Features"
	CommitTypeTitleBugFixes CommitTypeTitle = "Bug Fixes"
	CommitTypeTitleRefactor CommitTypeTitle = "Refactor"
	CommitTypeTitleCI       CommitTypeTitle = "CI"
)

type MarkdownTemplateContext struct {
	Sections []Section
}
type Section struct {
	Title   CommitTypeTitle
	Commits []*ConventionalCommit
}

func groupCommitsByTypeTitle(commits []string) []Section {
	grouped := make(map[string][]*ConventionalCommit)

	for _, c := range commits {
		conventionalCommit, valid := ParseConventionalCommit(c)
		// Skip invalid non-conventional commits
		if !valid {
			continue
		}
		grouped[conventionalCommit.Type] = append(grouped[conventionalCommit.Type], conventionalCommit)
	}

	var sections []Section
	for _, entry := range getDefaultTypeOrdering() {
		sections = append(sections, Section{Title: entry.Title, Commits: grouped[entry.Type]})
	}
	return sections
}

func getDefaultTypeOrdering() []TypeTuple {
	return []TypeTuple{
		{Type: "feat", Title: CommitTypeTitleFeatures},
		{Type: "fix", Title: CommitTypeTitleBugFixes},
		{Type: "refactor", Title: CommitTypeTitleRefactor},
		{Type: "ci", Title: CommitTypeTitleCI},
	}
}

type TypeTuple struct {
	Type  string
	Title CommitTypeTitle
}

type ConventionalCommit struct {
	Type        string
	Scope       string
	Description string
	Body        string
	Footers     []*ConventionalCommitFooter
}
type ConventionalCommitFooter struct {
	Trailer     string
	Description string
}

func ParseConventionalCommit(commit string) (*ConventionalCommit, bool) {
	sections := strings.Split(commit, "\n")

	firstLineFormat := regexp.MustCompile(`(?P<type>[a-zA-Z]+)(?:\((?P<scope>[a-zA-Z]+)\))?: (?P<description>.+)`)
	firstLineMatches := firstLineFormat.FindStringSubmatch(sections[0])
	if len(firstLineMatches) < 4 {
		return nil, false
	}
	conventionalCommit := &ConventionalCommit{
		Type:        firstLineMatches[firstLineFormat.SubexpIndex("type")],
		Description: firstLineMatches[firstLineFormat.SubexpIndex("description")],
		Scope:       firstLineMatches[firstLineFormat.SubexpIndex("scope")],
	}

	footerFormat := regexp.MustCompile(`(?P<trailer>[[:ascii:]]+): (?P<description>.+)`)
	for i := 2; i < len(sections); i++ {
		footerMatches := footerFormat.FindStringSubmatch(sections[i])
		if len(footerMatches) == 3 {
			footer := &ConventionalCommitFooter{
				Trailer:     footerMatches[footerFormat.SubexpIndex("trailer")],
				Description: footerMatches[footerFormat.SubexpIndex("description")],
			}
			conventionalCommit.Footers = append(conventionalCommit.Footers, footer)
		} else {
			conventionalCommit.Body = conventionalCommit.Body + sections[i] + "\n"
		}
	}
	conventionalCommit.Body = strings.Trim(conventionalCommit.Body, "\n")

	return conventionalCommit, true
}
