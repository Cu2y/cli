package list

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/run/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

const (
	defaultLimit = 10
)

type ListOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)

	Limit int
}

// TODO
// --state=(pending,pass,fail,etc)
// --active - pending
// --workflow - filter by workflow name

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recent workflow runs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if opts.Limit < 1 {
				return &cmdutil.FlagError{Err: fmt.Errorf("invalid limit: %v", opts.Limit)}
			}

			if runF != nil {
				return runF(opts)
			}

			return listRun(opts)
		},
	}

	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", defaultLimit, "Maximum number of runs to fetch")

	return cmd
}

type RunsPayload struct {
	TotalCount   int          `json:"total_count"`
	WorkflowRuns []shared.Run `json:"workflow_runs"`
}

func listRun(opts *ListOptions) error {
	baseRepo, err := opts.BaseRepo()
	if err != nil {
		// TODO better err handle
		return err
	}

	c, err := opts.HttpClient()
	if err != nil {
		// TODO better error handle
		return err
	}
	client := api.NewClientFromHTTP(c)

	runs, err := getRuns(client, baseRepo, opts.Limit)
	if err != nil {
		// TODO better error handle
		return err
	}

	tp := utils.NewTablePrinter(opts.IO)

	cs := opts.IO.ColorScheme()

	for _, run := range runs {
		idStr := cs.Cyan(fmt.Sprintf("%d", run.ID))
		// TODO nontty
		tp.AddField(shared.Symbol(cs, run.Status, run.Conclusion), nil, nil)
		tp.AddField(run.Name, nil, nil)
		tp.AddField(cs.Bold(run.HeadBranch), nil, nil)
		tp.AddField(string(run.Event), nil, nil)
		tp.AddField(fmt.Sprintf("(ID: %s)", idStr), nil, nil)
		// TODO can i sub updated at and created at to get elapsed?
		tp.EndRow()
	}

	err = tp.Render()
	if err != nil {
		// TODO better error handle
		return err
	}

	return nil
}

func getRuns(client *api.Client, repo ghrepo.Interface, limit int) ([]shared.Run, error) {
	perPage := limit
	page := 1
	if limit > 100 {
		perPage = 100
	}

	runs := []shared.Run{}

	for len(runs) < limit {
		var result RunsPayload

		path := fmt.Sprintf("repos/%s/actions/runs?per_page=%d&page=%d", ghrepo.FullName(repo), perPage, page)

		err := client.REST(repo.RepoHost(), "GET", path, nil, &result)
		if err != nil {
			// TODO better err handle
			return nil, err
		}

		if len(result.WorkflowRuns) == 0 {
			break
		}

		for _, run := range result.WorkflowRuns {
			runs = append(runs, run)
			if len(runs) == limit {
				break
			}
		}
		page++
	}

	return runs, nil
}
