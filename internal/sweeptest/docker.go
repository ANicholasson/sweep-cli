package sweeptest

import "github.com/ANicholasson/sweep-cli/internal/domain"

// Docker is the fake of the port a Scan reads Docker data through. The
// zero value reports it unread.
type Docker struct {
	Answer domain.DockerData
}

// Data returns the Docker data the test arranged.
func (d Docker) Data() domain.DockerData { return d.Answer }
