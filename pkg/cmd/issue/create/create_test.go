package create

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/internal/run"
	prShared "github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/cli/cli/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func runCommand(rt http.RoundTripper, isTTY bool, cli string) (*test.CmdOut, error) {
	return runCommandWithRootDirOverridden(rt, isTTY, cli, "")
}

func runCommandWithRootDirOverridden(rt http.RoundTripper, isTTY bool, cli string, rootDir string) (*test.CmdOut, error) {
	io, _, stdout, stderr := iostreams.Test()
	io.SetStdoutTTY(isTTY)
	io.SetStdinTTY(isTTY)
	io.SetStderrTTY(isTTY)

	factory := &cmdutil.Factory{
		IOStreams: io,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: rt}, nil
		},
		Config: func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
	}

	cmd := NewCmdCreate(factory, func(opts *CreateOptions) error {
		opts.RootDirOverride = rootDir
		return createRun(opts)
	})

	argv, err := shlex.Split(cli)
	if err != nil {
		return nil, err
	}
	cmd.SetArgs(argv)

	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(ioutil.Discard)
	cmd.SetErr(ioutil.Discard)

	_, err = cmd.ExecuteC()
	return &test.CmdOut{
		OutBuf: stdout,
		ErrBuf: stderr,
	}, err
}

func TestIssueCreate_nontty_error(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	_, err := runCommand(http, false, `-t hello`)
	assert.EqualError(t, err, "must provide --title and --body when not running interactively")
}

func TestIssueCreate(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"id": "REPOID",
				"hasIssuesEnabled": true
			} } }`),
	)
	http.Register(
		httpmock.GraphQL(`mutation IssueCreate\b`),
		httpmock.GraphQLMutation(`
				{ "data": { "createIssue": { "issue": {
					"URL": "https://github.com/OWNER/REPO/issues/12"
				} } } }`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["repositoryId"], "REPOID")
				assert.Equal(t, inputs["title"], "hello")
				assert.Equal(t, inputs["body"], "cash rules everything around me")
			}),
	)

	output, err := runCommand(http, true, `-t hello -b "cash rules everything around me"`)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	assert.Equal(t, "https://github.com/OWNER/REPO/issues/12\n", output.String())
}

func TestIssueCreate_recover(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"id": "REPOID",
				"hasIssuesEnabled": true
			} } }`))
	http.Register(
		httpmock.GraphQL(`query RepositoryResolveMetadataIDs\b`),
		httpmock.StringResponse(`
		{ "data": {
			"u000": { "login": "MonaLisa", "id": "MONAID" },
			"repository": {
				"l000": { "name": "bug", "id": "BUGID" },
				"l001": { "name": "TODO", "id": "TODOID" }
			}
		} }
		`))
	http.Register(
		httpmock.GraphQL(`mutation IssueCreate\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "createIssue": { "issue": {
			"URL": "https://github.com/OWNER/REPO/issues/12"
		} } } }
	`, func(inputs map[string]interface{}) {
			assert.Equal(t, "recovered title", inputs["title"])
			assert.Equal(t, "recovered body", inputs["body"])
			assert.Equal(t, []interface{}{"BUGID", "TODOID"}, inputs["labelIds"])
		}))

	as, teardown := prompt.InitAskStubber()
	defer teardown()

	as.Stub([]*prompt.QuestionStub{
		{
			Name:    "Title",
			Default: true,
		},
	})
	as.Stub([]*prompt.QuestionStub{
		{
			Name:    "Body",
			Default: true,
		},
	})
	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "confirmation",
			Value: 0,
		},
	})

	tmpfile, err := ioutil.TempFile(os.TempDir(), "testrecover*")
	assert.NoError(t, err)

	state := prShared.IssueMetadataState{
		Title:  "recovered title",
		Body:   "recovered body",
		Labels: []string{"bug", "TODO"},
	}

	data, err := json.Marshal(state)
	assert.NoError(t, err)

	_, err = tmpfile.Write(data)
	assert.NoError(t, err)

	args := fmt.Sprintf("--recover '%s'", tmpfile.Name())

	output, err := runCommandWithRootDirOverridden(http, true, args, "")
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	assert.Equal(t, "https://github.com/OWNER/REPO/issues/12\n", output.String())
}

func TestIssueCreate_nonLegacyTemplate(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"id": "REPOID",
				"hasIssuesEnabled": true
			} } }`),
	)
	http.Register(
		httpmock.GraphQL(`query IssueTemplates\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": { "issueTemplates": [
				{ "name": "Bug report",
				  "body": "Does not work :((" },
				{ "name": "Submit a request",
				  "body": "I have a suggestion for an enhancement" }
			] } } }`),
	)
	http.Register(
		httpmock.GraphQL(`mutation IssueCreate\b`),
		httpmock.GraphQLMutation(`
			{ "data": { "createIssue": { "issue": {
				"URL": "https://github.com/OWNER/REPO/issues/12"
			} } } }`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["repositoryId"], "REPOID")
				assert.Equal(t, inputs["title"], "hello")
				assert.Equal(t, inputs["body"], "I have a suggestion for an enhancement")
			}),
	)

	as, teardown := prompt.InitAskStubber()
	defer teardown()

	// template
	as.StubOne(1)
	// body
	as.Stub([]*prompt.QuestionStub{
		{
			Name:    "Body",
			Default: true,
		},
	}) // body
	// confirm
	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "confirmation",
			Value: 0,
		},
	})

	output, err := runCommandWithRootDirOverridden(http, true, `-t hello`, "./fixtures/repoWithNonLegacyIssueTemplates")
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	assert.Equal(t, "https://github.com/OWNER/REPO/issues/12\n", output.String())
}

func TestIssueCreate_continueInBrowser(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"id": "REPOID",
				"hasIssuesEnabled": true
			} } }`),
	)

	as, teardown := prompt.InitAskStubber()
	defer teardown()

	// title
	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "Title",
			Value: "hello",
		},
	})
	// confirm
	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "confirmation",
			Value: 1,
		},
	})

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`https://github\.com`, 0, "", func(args []string) {
		url := strings.ReplaceAll(args[len(args)-1], "^", "")
		assert.Equal(t, "https://github.com/OWNER/REPO/issues/new?body=body&title=hello", url)
	})

	output, err := runCommand(http, true, `-b body`)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, heredoc.Doc(`

		Creating issue in OWNER/REPO

		Opening github.com/OWNER/REPO/issues/new in your browser.
	`), output.Stderr())
}

func TestIssueCreate_continueInBrowser(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": {
			"id": "REPOID",
			"hasIssuesEnabled": true
		} } }
	`))

	as, teardown := prompt.InitAskStubber()
	defer teardown()

	// title
	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "Title",
			Value: "hello",
		},
	})
	// confirm
	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "confirmation",
			Value: 1,
		},
	})

	var seenCmd *exec.Cmd
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmd = cmd
		return &test.OutputStub{}
	})
	defer restoreCmd()

	output, err := runCommand(http, true, `-b body`)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, heredoc.Doc(`
		
		Creating issue in OWNER/REPO
		
		Opening github.com/OWNER/REPO/issues/new in your browser.
	`), output.Stderr())

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := strings.ReplaceAll(seenCmd.Args[len(seenCmd.Args)-1], "^", "")
	assert.Equal(t, "https://github.com/OWNER/REPO/issues/new?body=body&title=hello", url)
}

func TestIssueCreate_metadata(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.StubRepoInfoResponse("OWNER", "REPO", "main")
	http.Register(
		httpmock.GraphQL(`query RepositoryResolveMetadataIDs\b`),
		httpmock.StringResponse(`
		{ "data": {
			"u000": { "login": "MonaLisa", "id": "MONAID" },
			"repository": {
				"l000": { "name": "bug", "id": "BUGID" },
				"l001": { "name": "TODO", "id": "TODOID" }
			}
		} }
		`))
	http.Register(
		httpmock.GraphQL(`query RepositoryMilestoneList\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "milestones": {
			"nodes": [
				{ "title": "GA", "id": "GAID" },
				{ "title": "Big One.oh", "id": "BIGONEID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	http.Register(
		httpmock.GraphQL(`query RepositoryProjectList\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "projects": {
			"nodes": [
				{ "name": "Cleanup", "id": "CLEANUPID" },
				{ "name": "Roadmap", "id": "ROADMAPID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	http.Register(
		httpmock.GraphQL(`query OrganizationProjectList\b`),
		httpmock.StringResponse(`
		{	"data": { "organization": null },
			"errors": [{
				"type": "NOT_FOUND",
				"path": [ "organization" ],
				"message": "Could not resolve to an Organization with the login of 'OWNER'."
			}]
		}
		`))
	http.Register(
		httpmock.GraphQL(`mutation IssueCreate\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "createIssue": { "issue": {
			"URL": "https://github.com/OWNER/REPO/issues/12"
		} } } }
	`, func(inputs map[string]interface{}) {
			assert.Equal(t, "TITLE", inputs["title"])
			assert.Equal(t, "BODY", inputs["body"])
			assert.Equal(t, []interface{}{"MONAID"}, inputs["assigneeIds"])
			assert.Equal(t, []interface{}{"BUGID", "TODOID"}, inputs["labelIds"])
			assert.Equal(t, []interface{}{"ROADMAPID"}, inputs["projectIds"])
			assert.Equal(t, "BIGONEID", inputs["milestoneId"])
			if v, ok := inputs["userIds"]; ok {
				t.Errorf("did not expect userIds: %v", v)
			}
			if v, ok := inputs["teamIds"]; ok {
				t.Errorf("did not expect teamIds: %v", v)
			}
		}))

	output, err := runCommand(http, true, `-t TITLE -b BODY -a monalisa -l bug -l todo -p roadmap -m 'big one.oh'`)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	assert.Equal(t, "https://github.com/OWNER/REPO/issues/12\n", output.String())
}

func TestIssueCreate_disabledIssues(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"id": "REPOID",
				"hasIssuesEnabled": false
			} } }`),
	)

	_, err := runCommand(http, true, `-t heres -b johnny`)
	if err == nil || err.Error() != "the 'OWNER/REPO' repository has disabled issues" {
		t.Errorf("error running command `issue create`: %v", err)
	}
}

func TestIssueCreate_web(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query UserCurrent\b`),
		httpmock.StringResponse(`
		{ "data": {
			"viewer": { "login": "MonaLisa" }
		} }
		`),
	)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`https://github\.com`, 0, "", func(args []string) {
		url := strings.ReplaceAll(args[len(args)-1], "^", "")
		assert.Equal(t, "https://github.com/OWNER/REPO/issues/new?assignees=MonaLisa", url)
	})

	output, err := runCommand(http, true, `--web -a @me`)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "Opening github.com/OWNER/REPO/issues/new in your browser.\n", output.Stderr())
}

func TestIssueCreate_webTitleBody(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`https://github\.com`, 0, "", func(args []string) {
		url := strings.ReplaceAll(args[len(args)-1], "^", "")
		assert.Equal(t, "https://github.com/OWNER/REPO/issues/new?body=mybody&title=mytitle", url)
	})

	output, err := runCommand(http, true, `-w -t mytitle -b mybody`)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "Opening github.com/OWNER/REPO/issues/new in your browser.\n", output.Stderr())
}

func TestIssueCreate_webTitleBodyAtMeAssignee(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query UserCurrent\b`),
		httpmock.StringResponse(`
		{ "data": {
			"viewer": { "login": "MonaLisa" }
		} }
		`),
	)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`https://github\.com`, 0, "", func(args []string) {
		url := strings.ReplaceAll(args[len(args)-1], "^", "")
		assert.Equal(t, "https://github.com/OWNER/REPO/issues/new?assignees=MonaLisa&body=mybody&title=mytitle", url)
	})

	output, err := runCommand(http, true, `-w -t mytitle -b mybody -a @me`)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "Opening github.com/OWNER/REPO/issues/new in your browser.\n", output.Stderr())
}

func TestIssueCreate_AtMeAssignee(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query UserCurrent\b`),
		httpmock.StringResponse(`
		{ "data": {
			"viewer": { "login": "MonaLisa" }
		} }
		`),
	)
	http.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": {
			"id": "REPOID",
			"hasIssuesEnabled": true
		} } }
	`))
	http.Register(
		httpmock.GraphQL(`query RepositoryResolveMetadataIDs\b`),
		httpmock.StringResponse(`
		{ "data": {
			"u000": { "login": "MonaLisa", "id": "MONAID" },
			"u001": { "login": "SomeOneElse", "id": "SOMEID" },
			"repository": {
				"l000": { "name": "bug", "id": "BUGID" },
				"l001": { "name": "TODO", "id": "TODOID" }
			}
		} }
		`),
	)
	http.Register(
		httpmock.GraphQL(`mutation IssueCreate\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "createIssue": { "issue": {
			"URL": "https://github.com/OWNER/REPO/issues/12"
		} } } }
	`, func(inputs map[string]interface{}) {
			assert.Equal(t, "hello", inputs["title"])
			assert.Equal(t, "cash rules everything around me", inputs["body"])
			assert.Equal(t, []interface{}{"MONAID", "SOMEID"}, inputs["assigneeIds"])
		}))

	output, err := runCommand(http, true, `-a @me -a someoneelse -t hello -b "cash rules everything around me"`)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	assert.Equal(t, "https://github.com/OWNER/REPO/issues/12\n", output.String())
}

func TestIssueCreate_webProject(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query RepositoryProjectList\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": { "projects": {
				"nodes": [
					{ "name": "Cleanup", "id": "CLEANUPID", "resourcePath": "/OWNER/REPO/projects/1" }
				],
				"pageInfo": { "hasNextPage": false }
			} } } }
			`))
	http.Register(
		httpmock.GraphQL(`query OrganizationProjectList\b`),
		httpmock.StringResponse(`
			{ "data": { "organization": { "projects": {
				"nodes": [
					{ "name": "Triage", "id": "TRIAGEID", "resourcePath": "/orgs/ORG/projects/1"  }
				],
				"pageInfo": { "hasNextPage": false }
			} } } }
			`))

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`https://github\.com`, 0, "", func(args []string) {
		url := strings.ReplaceAll(args[len(args)-1], "^", "")
		assert.Equal(t, "https://github.com/OWNER/REPO/issues/new?projects=OWNER%2FREPO%2F1&title=Title", url)
	})

	output, err := runCommand(http, true, `-w -t Title -p Cleanup`)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "Opening github.com/OWNER/REPO/issues/new in your browser.\n", output.Stderr())
}
