package changelog

import (
	"os"
	"path"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/google/go-cmp/cmp"
)

func TestParseConventionalCommit(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		input  string
		result *ConventionalCommit
	}{
		"commit message with description and breaking change footer": {
			input: `feat: allow provided config object to extend other configs

BREAKING CHANGE: ` + "`extends`" + ` key in config file is now used for extending other config files`,
			result: &ConventionalCommit{
				Type:        "feat",
				Description: "allow provided config object to extend other configs",
				Footers: []*ConventionalCommitFooter{
					{
						Trailer:     "BREAKING CHANGE",
						Description: "`extends` key in config file is now used for extending other config files",
					},
				},
			},
		},
		"commit message with no body": {
			input:  `docs: correct spelling of CHANGELOG`,
			result: &ConventionalCommit{Type: "docs", Description: "correct spelling of CHANGELOG"},
		},
		"commit message with scope and no body": {
			input:  `feat(lang): add Polish language`,
			result: &ConventionalCommit{Type: "feat", Scope: "lang", Description: "add Polish language"},
		},
		"commit message with scope and body": {
			input: `feat(lang): add Polish language

Lorem ipsum dolor sit amet, consectetur adipiscing elit.`,
			result: &ConventionalCommit{
				Type:        "feat",
				Scope:       "lang",
				Description: "add Polish language",
				Body:        "Lorem ipsum dolor sit amet, consectetur adipiscing elit.",
			},
		},
		"commit message with multi-paragraph body and multiple footers": {
			input: `fix: prevent racing of requests

Introduce a request id and a reference to latest request. Dismiss
incoming responses other than from latest request.

Remove timeouts which were used to mitigate the racing issue but are
obsolete now.

Reviewed-by: Z
Refs: #123`,
			result: &ConventionalCommit{
				Type:        "fix",
				Description: "prevent racing of requests",
				Body: `Introduce a request id and a reference to latest request. Dismiss
incoming responses other than from latest request.

Remove timeouts which were used to mitigate the racing issue but are
obsolete now.`,
				Footers: []*ConventionalCommitFooter{
					{
						Trailer:     "Reviewed-by",
						Description: "Z",
					},
					{
						Trailer:     "Refs",
						Description: "#123",
					},
				},
			},
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()
			got, valid := ParseConventionalCommit(test.input)
			if !valid {
				t.Fatalf("ParseConventionalCommit(%q) is: %+v", test.input, valid)
			}
			if !reflect.DeepEqual(got, test.result) {
				t.Fatalf("ParseConventionalCommit(%q)\n--- RETURNED ---\n%+v\n--- EXPECTED ---\n%+v", test.input, got, test.result)
			}
		})
	}
}

func TestGenerate(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		input  TestGenerateInput
		result string
	}{
		"default markdown changelog with multiple commits": {
			input: TestGenerateInput{
				fileCommitMapping: FileCommitMapping{
					"README.md": `feat(lang): add Polish language`,
					"main.go": `fix: prevent racing of requests

Introduce a request id and a reference to latest request. Dismiss
incoming responses other than from latest request.

Remove timeouts which were used to mitigate the racing issue but are
obsolete now.

verify: check request id inside logs`,
					"main.tf":       `refactor: rename terraform module`,
					"pipeline.yaml": `ci: implement autobahn pipeline`,
				},
			},
			result: `## [unreleased]

### Features
- *lang* add Polish language

### Bug Fixes
- prevent racing of requests

### Refactor
- rename terraform module

### CI
- implement autobahn pipeline
`,
		},
		"custom changelog template with multiple commits": {
			input: TestGenerateInput{
				fileCommitMapping: FileCommitMapping{
					"README.md": `feat(lang): add Polish language`,
					"main.go": `fix: prevent racing of requests

Introduce a request id and a reference to latest request. Dismiss
incoming responses other than from latest request.

Remove timeouts which were used to mitigate the racing issue but are
obsolete now.

verify: check request id inside logs
`,
					"main.tf":       `refactor: rename terraform module`,
					"pipeline.yaml": `ci: implement autobahn pipeline`,
				},
				changelogTemplate: `## [unreleased]
{{ range .Sections }}
### {{ .Title }}
{{ range .Commits -}}
- {{ if .Scope }}*{{ .Scope }}* {{ end }}{{ .Description }}
{{- range .Footers }}
    + {{ .Trailer }}: {{ .Description }}
{{- end }}
{{ end }}
{{- end }}`,
			},
			result: `## [unreleased]

### Features
- *lang* add Polish language

### Bug Fixes
- prevent racing of requests
    + verify: check request id inside logs

### Refactor
- rename terraform module

### CI
- implement autobahn pipeline
`,
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			testDir := "/tmp/" + strings.ReplaceAll(testName, " ", "_")
			if err := generateTestRepo(testDir, test.input.fileCommitMapping); err != nil {
				t.Fatalf("generateTestRepo(%q, %q) error: %+v", testDir, test.input, err)
			}

			got, err := Generate(testDir, test.input.changelogTemplate)
			if err != nil {
				t.Fatalf("Generate(%q) error: %+v", testDir, err)
			}
			if diff := cmp.Diff(test.result, got); diff != "" {
				t.Errorf("Generate(%q) mismatch (-want +got):\n%s", testDir, diff)
			}

			if err := os.RemoveAll(testDir); err != nil {
				t.Fatalf("os.RemoveAll(%q) error: %+v", testDir, err)
			}
		})
	}
}

type TestGenerateInput struct {
	fileCommitMapping FileCommitMapping
	changelogTemplate string `default:""`
}

type FileCommitMapping map[string]string

func generateTestRepo(repoPath string, fileCommitMapping FileCommitMapping) error {
	r, err := git.PlainInit(repoPath, false)
	if err != nil {
		return err
	}

	w, err := r.Worktree()
	if err != nil {
		return err
	}

	for fileName, cm := range fileCommitMapping {
		filePath := path.Join(repoPath, fileName)
		err := os.WriteFile(filePath, []byte(cm+"\n"), 0644)
		if err != nil {
			return err
		}

		_, err = w.Add(filePath)
		if err != nil {
			return err
		}

		_, err = w.Commit(cm, &git.CommitOptions{
			Author: &object.Signature{
				Name:  "John Doe",
				Email: "john@example.com",
				When:  time.Now(),
			},
		})
		if err != nil {
			return err
		}
	}

	return nil
}
