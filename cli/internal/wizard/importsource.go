// Package wizard holds small interactive prompts used by higher-level
// commands. The functions here are designed to take an explicit io.Reader
// and io.Writer so tests can drive them deterministically.
package wizard

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/nitin-1926/ccpm/internal/defaultclaude"
)

// Source enumerates where a new profile can pull pre-existing assets from.
type Source int

const (
	SourceScratch Source = iota // start empty — no import
	SourceDefault                // import from ~/.claude
	SourceProfile                // import from another ccpm profile
)

// Decision describes what the user picked in the wizard.
type Decision struct {
	Source      Source
	ProfileName string // only set when Source == SourceProfile
	Targets     []defaultclaude.Target
}

// PromptImportSource runs an interactive prompt asking the user whether the
// new profile should start empty, import from ~/.claude, or clone another
// profile's assets. Non-interactive callers (no TTY / empty input) receive
// SourceScratch so add never blocks CI.
func PromptImportSource(r io.Reader, w io.Writer, existingProfiles []string, hasDefault bool) (Decision, error) {
	reader := bufio.NewReader(r)

	options := []string{"Start empty (no import)"}
	sourceMap := []Source{SourceScratch}
	if hasDefault {
		options = append(options, "Import from ~/.claude (your current Claude Code setup)")
		sourceMap = append(sourceMap, SourceDefault)
	}
	if len(existingProfiles) > 0 {
		options = append(options, "Copy from another ccpm profile")
		sourceMap = append(sourceMap, SourceProfile)
	}

	// Nothing to import from — quietly return scratch.
	if len(options) == 1 {
		return Decision{Source: SourceScratch}, nil
	}

	fmt.Fprintln(w, "Where should this profile inherit assets from?")
	for i, opt := range options {
		fmt.Fprintf(w, "  %d) %s\n", i+1, opt)
	}
	fmt.Fprintf(w, "Enter choice [1-%d, default 1]: ", len(options))

	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return Decision{}, fmt.Errorf("reading choice: %w", err)
	}
	choice := strings.TrimSpace(line)
	if choice == "" {
		return Decision{Source: SourceScratch}, nil
	}
	idx, err := strconv.Atoi(choice)
	if err != nil || idx < 1 || idx > len(options) {
		return Decision{}, fmt.Errorf("invalid choice %q", choice)
	}
	picked := sourceMap[idx-1]

	decision := Decision{Source: picked, Targets: defaultclaude.DefaultTargets()}

	if picked == SourceProfile {
		name, err := promptProfileName(reader, w, existingProfiles)
		if err != nil {
			return Decision{}, err
		}
		decision.ProfileName = name
	}

	return decision, nil
}

func promptProfileName(reader *bufio.Reader, w io.Writer, existing []string) (string, error) {
	fmt.Fprintln(w, "Available profiles:")
	for i, n := range existing {
		fmt.Fprintf(w, "  %d) %s\n", i+1, n)
	}
	fmt.Fprint(w, "Pick a profile to copy from (number or name): ")

	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("reading profile name: %w", err)
	}
	answer := strings.TrimSpace(line)
	if answer == "" {
		return "", fmt.Errorf("no profile selected")
	}

	if idx, err := strconv.Atoi(answer); err == nil {
		if idx < 1 || idx > len(existing) {
			return "", fmt.Errorf("invalid profile index %d", idx)
		}
		return existing[idx-1], nil
	}

	for _, n := range existing {
		if n == answer {
			return n, nil
		}
	}
	return "", fmt.Errorf("profile %q not found", answer)
}
