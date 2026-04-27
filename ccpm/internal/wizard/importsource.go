// Package wizard holds small interactive prompts used by higher-level
// commands. The functions here are designed to take an explicit io.Reader
// and io.Writer so tests can drive them deterministically.
package wizard

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/defaultclaude"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/picker"
)

// Source enumerates where a new profile can pull pre-existing assets from.
type Source int

const (
	SourceScratch Source = iota // start empty — no import
	SourceDefault               // import from ~/.claude
	SourceProfile               // import from another ccpm profile
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
//
// The r/w parameters are retained for the numeric-menu fallback used when
// stdin is not a TTY; in interactive mode the real terminal is driven by the
// picker library regardless of these writers.
func PromptImportSource(r io.Reader, w io.Writer, existingProfiles []string, hasDefault bool) (Decision, error) {
	var (
		sourceOpts []picker.Option
		sourceMap  []Source
		labels     []string
	)
	sourceOpts = append(sourceOpts, picker.Option{Value: "scratch", Label: "Start empty", Description: "no import"})
	sourceMap = append(sourceMap, SourceScratch)
	labels = append(labels, "Start empty (no import)")
	if hasDefault {
		sourceOpts = append(sourceOpts, picker.Option{Value: "default", Label: "Import from ~/.claude", Description: "your current Claude Code setup"})
		sourceMap = append(sourceMap, SourceDefault)
		labels = append(labels, "Import from ~/.claude (your current Claude Code setup)")
	}
	if len(existingProfiles) > 0 {
		sourceOpts = append(sourceOpts, picker.Option{Value: "profile", Label: "Copy from another ccpm profile"})
		sourceMap = append(sourceMap, SourceProfile)
		labels = append(labels, "Copy from another ccpm profile")
	}

	if len(sourceOpts) == 1 {
		return Decision{Source: SourceScratch}, nil
	}

	// Single shared reader so the numeric-menu fallback can consume
	// multiple lines (source choice, then profile name) without losing
	// buffered bytes.
	reader := bufio.NewReader(r)

	choice, err := picker.Select("Where should this profile inherit assets from?", sourceOpts)
	if err != nil && !errors.Is(err, picker.ErrNonInteractive) {
		return Decision{}, err
	}

	var picked Source
	if err == nil {
		for i, o := range sourceOpts {
			if o.Value == choice {
				picked = sourceMap[i]
				break
			}
		}
	} else {
		// Non-interactive fallback: keep the numeric menu for scripts.
		p, err := promptSourceTextMenu(reader, w, labels, sourceMap)
		if err != nil {
			return Decision{}, err
		}
		picked = p
	}

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

func promptSourceTextMenu(reader *bufio.Reader, w io.Writer, labels []string, sourceMap []Source) (Source, error) {
	fmt.Fprintln(w, "Where should this profile inherit assets from?")
	for i, opt := range labels {
		fmt.Fprintf(w, "  %d) %s\n", i+1, opt)
	}
	fmt.Fprintf(w, "Enter choice [1-%d, default 1]: ", len(labels))

	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return 0, fmt.Errorf("reading choice: %w", err)
	}
	raw := strings.TrimSpace(line)
	if raw == "" {
		return SourceScratch, nil
	}
	idx, err := strconv.Atoi(raw)
	if err != nil || idx < 1 || idx > len(labels) {
		return 0, fmt.Errorf("invalid choice %q", raw)
	}
	return sourceMap[idx-1], nil
}

func promptProfileName(reader *bufio.Reader, w io.Writer, existing []string) (string, error) {
	if picker.IsInteractive() {
		opts := make([]picker.Option, len(existing))
		for i, n := range existing {
			opts[i] = picker.Option{Value: n, Label: n}
		}
		return picker.Select("Pick a profile to copy from", opts)
	}

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
