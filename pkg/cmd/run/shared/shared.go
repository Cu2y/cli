package shared

import (
	"time"

	"github.com/cli/cli/pkg/iostreams"
)

const (
	// Run statuses
	Queued     = "queued"
	Completed  = "completed"
	InProgress = "in_progress"
	Requested  = "requested"
	Waiting    = "waiting"

	// Run conclusions
	ActionRequred  = "action_required"
	Cancelled      = "cancelled"
	Failure        = "failure"
	Neutral        = "neutral"
	Skipped        = "skipped"
	Stale          = "stale"
	StartupFailure = "startup_failure"
	Success        = "success"
	TimedOut       = "timed_out"

	// TODO events
)

type Status string
type Conclusion string
type RunEvent string

type Run struct {
	Name       string
	CreatedAt  time.Time `json:"created_at"`
	Status     Status
	Conclusion Conclusion
	Event      RunEvent
	ID         int
	HeadBranch string `json:"head_branch"`
	JobsURL    string `json:"jobs_url"`
}

func Symbol(cs *iostreams.ColorScheme, status Status, conclusion Conclusion) string {
	if status == Completed {
		switch conclusion {
		case Success:
			return cs.SuccessIcon()
		case Skipped, Cancelled, Neutral:
			return cs.SuccessIconWithColor(cs.Gray)
		default:
			return cs.FailureIcon()
		}
	}

	return cs.Yellow("-")
}
