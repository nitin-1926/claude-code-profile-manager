//go:build darwin

package services

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
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
func (s *HealthService) Doctor() (HealthResult, error) {
	bin := findCCPM()
	if bin == "" {
		return HealthResult{Available: false, Error: "ccpm CLI not found on PATH"}, nil
	}
	cmd := exec.Command(bin, "doctor")
	cmd.Env = append(envWithoutColor(), "NO_COLOR=1")
	out, err := cmd.CombinedOutput()
	res := HealthResult{Available: true, CCPMPath: bin, Output: ansiRE.ReplaceAllString(string(out), "")}
	if err != nil {
		// doctor exits non-zero when it finds problems; that's not a tool failure
		res.Error = strings.TrimSpace(err.Error())
	}
	return res, nil
}
