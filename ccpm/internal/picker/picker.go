// Package picker wraps charmbracelet/huh to offer single-select, multi-select,
// and yes/no prompts with a consistent look across ccpm commands.
//
// Every function first checks whether stdin is a TTY. In a non-interactive
// context (CI, piped input, tests) it returns ErrNonInteractive so the caller
// can fall back to its existing required-flag error and keep scripts working.
package picker

import (
	"errors"
	"os"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"
)

// ErrNonInteractive is returned when prompting is attempted without a TTY.
var ErrNonInteractive = errors.New("picker: non-interactive terminal")

// Option is one entry in a Select or MultiSelect prompt.
type Option struct {
	Value       string
	Label       string
	Description string
}

// Select presents a single-select list. Returns the chosen option's Value.
func Select(title string, options []Option) (string, error) {
	if !IsInteractive() {
		return "", ErrNonInteractive
	}
	opts := make([]huh.Option[string], len(options))
	for i, o := range options {
		label := o.Label
		if o.Description != "" {
			label = o.Label + "  \033[2m—\033[0m " + o.Description
		}
		opts[i] = huh.NewOption(label, o.Value)
	}

	var choice string
	err := huh.NewSelect[string]().
		Title(title).
		Options(opts...).
		Value(&choice).
		Run()
	if err != nil {
		return "", err
	}
	return choice, nil
}

// MultiSelect presents a multi-select list (spacebar toggles). Returns the
// Values of the chosen options in the order they appeared.
func MultiSelect(title string, options []Option, defaults []string) ([]string, error) {
	if !IsInteractive() {
		return nil, ErrNonInteractive
	}
	opts := make([]huh.Option[string], len(options))
	for i, o := range options {
		label := o.Label
		if o.Description != "" {
			label = o.Label + "  \033[2m—\033[0m " + o.Description
		}
		opt := huh.NewOption(label, o.Value)
		for _, d := range defaults {
			if d == o.Value {
				opt = opt.Selected(true)
				break
			}
		}
		opts[i] = opt
	}

	var chosen []string
	err := huh.NewMultiSelect[string]().
		Title(title).
		Description("space to toggle, enter to confirm").
		Options(opts...).
		Value(&chosen).
		Run()
	if err != nil {
		return nil, err
	}
	return chosen, nil
}

// Confirm presents a yes/no prompt with the given default.
func Confirm(prompt string, def bool) (bool, error) {
	if !IsInteractive() {
		return def, ErrNonInteractive
	}
	v := def
	err := huh.NewConfirm().
		Title(prompt).
		Value(&v).
		Run()
	if err != nil {
		return def, err
	}
	return v, nil
}

// IsInteractive reports whether prompts should be shown. Honors CCPM_NO_TTY
// as a test / scripting override.
func IsInteractive() bool {
	if os.Getenv("CCPM_NO_TTY") != "" {
		return false
	}
	return term.IsTerminal(int(os.Stdin.Fd()))
}
