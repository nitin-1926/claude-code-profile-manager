//go:build darwin

package services

import (
	"os"
	"regexp"
	"strings"
	"time"
)

// envWithoutColor returns the current environment with color-forcing vars stripped.
func envWithoutColor() []string {
	var out []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "FORCE_COLOR=") || strings.HasPrefix(kv, "CLICOLOR_FORCE=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

type HealthResult struct {
	Available bool   `json:"available"` // ccpm binary found + ran
	CCPMPath  string `json:"ccpmPath"`
	Output    string `json:"output"`
	Error     string `json:"error"`
}

// HealthService surfaces `ccpm doctor` (the hybrid read-of-a-write-tool path).
type HealthService struct{}

func NewHealth() *HealthService { return &HealthService{} }

// Doctor runs `ccpm doctor` with color disabled and returns its plain output.
// Bounded so a stalled doctor can't hang the Health tab forever. doctor exits
// non-zero when it finds problems; that lands in Error, not as a tool failure.
func (s *HealthService) Doctor() (HealthResult, error) {
	r, _ := execCCPM(30*time.Second, "doctor")
	if r.CCPMPath == "" {
		return HealthResult{Available: false, Error: r.Error}, nil
	}
	return HealthResult{Available: true, CCPMPath: r.CCPMPath, Output: r.Output, Error: r.Error}, nil
}
