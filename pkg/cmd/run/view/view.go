package view

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/run/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type ViewOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	RunID string
	// TODO verbosity?

	Now func() time.Time
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Now:        time.Now,
	}
	cmd := &cobra.Command{
		Use:   "view",
		Short: "View a summary of a workflow run",
		// TODO examples?
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			opts.RunID = args[0]

			if runF != nil {
				return runF(opts)
			}
			return listView(opts)
		},
	}

	return cmd
}

func listView(opts *ViewOptions) error {
	opts.IO.StartProgressIndicator()
	c, err := opts.HttpClient()
	if err != nil {
		// TODO error handle
		return err
	}
	client := api.NewClientFromHTTP(c)

	repo, err := opts.BaseRepo()
	if err != nil {
		// TODO error handle
		return err
	}

	run, err := getRun(client, repo, opts.RunID)
	if err != nil {
		// TODO error handle
		return err
	}

	jobs, err := getJobs(client, repo, *run)
	if err != nil {
		// TODO error handle
		return err
	}

	var annotations []Annotation

	var annotationErr error
	var as []Annotation
	for _, job := range jobs {
		as, annotationErr = getAnnotations(client, repo, job)
		if annotationErr != nil {
			break
		}

		for _, a := range as {
			annotations = append(annotations, a)
		}
	}

	if annotationErr != nil {
		// TODO handle error
		return annotationErr
	}

	opts.IO.StopProgressIndicator()
	err = renderRun(*opts, *run, jobs, annotations)
	if err != nil {
		// TODO handle error
		return err
	}

	return nil
}

func titleForRun(cs *iostreams.ColorScheme, run shared.Run) string {
	// TODO how to obtain? i can get a SHA but it's not immediately clear how to get from sha -> pr
	// without a ton of hops
	prID := ""

	return fmt.Sprintf("%s %s%s",
		cs.Bold(run.HeadBranch),
		run.Name,
		prID)
}

// TODO consider context struct for all this:

func renderRun(opts ViewOptions, run shared.Run, jobs []Job, annotations []Annotation) error {
	out := opts.IO.Out
	cs := opts.IO.ColorScheme()

	title := titleForRun(cs, run)
	symbol := shared.Symbol(cs, run.Status, run.Conclusion)
	id := cs.Cyan(fmt.Sprintf("%d", run.ID))

	fmt.Fprintf(out, "%s %s · %s\n", symbol, title, id)

	ago := opts.Now().Sub(run.CreatedAt)

	fmt.Fprintf(out, "Triggered via %s %s\n", run.Event, utils.FuzzyAgo(ago))
	fmt.Fprintln(out)
	fmt.Fprintln(out, cs.Bold("JOBS"))

	tp := utils.NewTablePrinter(opts.IO)

	for _, job := range jobs {
		symbol := shared.Symbol(cs, job.Status, job.Conclusion)
		id := cs.Cyan(fmt.Sprintf("%d", job.ID))
		tp.AddField(fmt.Sprintf("%s %s", symbol, job.Name), nil, nil)
		tp.AddField(fmt.Sprintf("ID: %s", id), nil, nil)
		tp.EndRow()
	}

	err := tp.Render()
	if err != nil {
		return err
	}

	if len(annotations) == 0 {
		return nil
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, cs.Bold("ANNOTATIONS"))

	for _, a := range annotations {
		fmt.Fprintf(out, "%s %s\n", a.Symbol(cs), a.Message)
		fmt.Fprintln(out, cs.Gray(fmt.Sprintf("%s: %s#%d\n",
			a.JobName, a.Path, a.StartLine)))
	}

	return nil
}
