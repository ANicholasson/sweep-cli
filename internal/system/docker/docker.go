// Package docker asks the docker program how much space images and build
// cache hold. It only reads. The only command it runs is docker system df,
// which changes nothing.
package docker

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"math"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ANicholasson/sweep-cli/internal/domain"
	"github.com/ANicholasson/sweep-cli/internal/domain/scan"
)

// Local is the docker program on this machine. The environment is left as
// it is, so a Docker host set there is the one that is asked.
type Local struct {
	// Bin is the program to run. Empty runs docker from PATH.
	Bin string
}

var _ scan.Docker = Local{}

// dfFormat is the docker system df template that prints one JSON object
// per row. Size in that object is the same total the table shows, written
// in docker's own units.
const dfFormat = "{{json .}}"

// Data reports the Images and Build Cache totals from docker system df,
// added together. A missing program, a daemon that is not running, or an
// answer that is not those two totals comes back unread, with why.
func (l Local) Data() domain.DockerData {
	bin := l.Bin
	if bin == "" {
		bin = "docker"
	}
	cmd := exec.Command(bin, "system", "df", "--format", dfFormat)
	out, err := cmd.Output()
	if err != nil {
		return domain.Unread(why(err))
	}
	size, err := parseDF(out)
	if err != nil {
		return domain.Unread("docker did not answer")
	}
	return domain.Reported(size)
}

func why(err error) string {
	if missing(err) {
		return "docker is missing"
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && daemonDown(string(exit.Stderr)) {
		return "docker is not running"
	}
	return "docker did not answer"
}

func missing(err error) bool {
	var run *exec.Error
	if errors.As(err, &run) && errors.Is(run.Err, exec.ErrNotFound) {
		return true
	}
	// A name on PATH that cannot be found is ErrNotFound. A Bin set to a
	// path that is not there is a path error instead.
	return errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist)
}

func daemonDown(msg string) bool {
	return strings.Contains(msg, "Cannot connect to the Docker daemon") ||
		strings.Contains(msg, "Is the docker daemon running") ||
		strings.Contains(msg, "error during connect")
}

// parseDF adds the Images and Build Cache sizes. Both rows must be
// present. Every other row is ignored. Size text is what docker system df
// prints, such as 1.5kB or 2MB, in decimal units of 1000 except for the
// KiB family.
func parseDF(out []byte) (domain.Size, error) {
	var (
		images, cache       int64
		sawImages, sawCache bool
	)
	for _, line := range bytes.Split(out, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var row struct {
			Type string
			Size string
		}
		if err := json.Unmarshal(line, &row); err != nil {
			return 0, err
		}
		if row.Type != "Images" && row.Type != "Build Cache" {
			continue
		}
		n, err := parseHuman(row.Size)
		if err != nil {
			return 0, err
		}
		if row.Type == "Images" {
			if sawImages {
				return 0, errors.New("duplicate row")
			}
			images, sawImages = n, true
			continue
		}
		if sawCache {
			return 0, errors.New("duplicate row")
		}
		cache, sawCache = n, true
	}
	if !sawImages || !sawCache {
		return 0, errors.New("missing row")
	}
	if images < 0 || cache < 0 || images > math.MaxInt64-cache {
		return 0, errors.New("overflow")
	}
	return domain.Size(images + cache), nil
}

// parseHuman reads a size as docker system df prints it. The number is
// rounded to the nearest byte.
func parseHuman(s string) (int64, error) {
	s = strings.TrimSpace(s)
	i := len(s)
	for i > 0 {
		c := s[i-1]
		if (c >= '0' && c <= '9') || c == '.' {
			break
		}
		i--
	}
	number, unit := strings.TrimSpace(s[:i]), strings.TrimSpace(s[i:])
	value, err := strconv.ParseFloat(number, 64)
	if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, errors.New("size")
	}
	mult, ok := humanUnit[unit]
	if !ok {
		return 0, errors.New("unit")
	}
	bytes := math.Round(value * mult)
	if bytes < 0 || bytes > math.MaxInt64 {
		return 0, errors.New("overflow")
	}
	return int64(bytes), nil
}

// humanUnit is the unit table docker's own size printer uses: decimal
// powers of 1000, and the binary powers for when a size already says KiB.
var humanUnit = map[string]float64{
	"B":   1,
	"kB":  1e3,
	"KB":  1e3,
	"MB":  1e6,
	"GB":  1e9,
	"TB":  1e12,
	"PB":  1e15,
	"KiB": 1 << 10,
	"MiB": 1 << 20,
	"GiB": 1 << 30,
	"TiB": 1 << 40,
	"PiB": 1 << 50,
}
