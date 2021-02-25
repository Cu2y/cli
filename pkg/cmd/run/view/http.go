package view

import (
	"fmt"
	"net/url"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/run/shared"
	"github.com/cli/cli/pkg/iostreams"
)

func getRun(client *api.Client, repo ghrepo.Interface, runID string) (*shared.Run, error) {
	var result shared.Run

	path := fmt.Sprintf("repos/%s/actions/runs/%s", ghrepo.FullName(repo), runID)

	err := client.REST(repo.RepoHost(), "GET", path, nil, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

type Job struct {
	ID         int
	Status     shared.Status
	Conclusion shared.Conclusion
	Name       string
}

type JobsPayload struct {
	Jobs []Job
}

func getJobs(client *api.Client, repo ghrepo.Interface, run shared.Run) ([]Job, error) {
	var result JobsPayload
	parsed, err := url.Parse(run.JobsURL)
	if err != nil {
		return nil, err
	}

	err = client.REST(repo.RepoHost(), "GET", parsed.Path[1:], nil, &result)
	if err != nil {
		return nil, err
	}
	return result.Jobs, nil
}

type Annotation struct {
	JobName   string
	Message   string
	Path      string
	Level     string `json:"annotation_level"`
	StartLine int    `json:"start_line"`
}

func (a Annotation) Symbol(cs *iostreams.ColorScheme) string {
	// TODO types for levels
	switch a.Level {
	case "failure":
		return cs.FailureIcon()
	case "warning":
		return cs.Yellow("!")
	default:
		return "TODO"
	}
}

type CheckRun struct {
	ID int
}

func getAnnotations(client *api.Client, repo ghrepo.Interface, job Job) ([]Annotation, error) {

	var result []*Annotation

	path := fmt.Sprintf("repos/%s/check-runs/%d/annotations", ghrepo.FullName(repo), job.ID)

	err := client.REST(repo.RepoHost(), "GET", path, nil, &result)
	if err != nil {
		return nil, err
	}

	out := []Annotation{}

	for _, annotation := range result {
		annotation.JobName = job.Name
		out = append(out, *annotation)
	}

	return out, nil
}
