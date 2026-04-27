package picker

import (
	"errors"
	"testing"
)

func TestNonInteractiveFallback(t *testing.T) {
	t.Setenv("CCPM_NO_TTY", "1")

	if IsInteractive() {
		t.Fatalf("IsInteractive should be false with CCPM_NO_TTY")
	}

	if _, err := Select("pick", []Option{{Value: "a", Label: "A"}}); !errors.Is(err, ErrNonInteractive) {
		t.Fatalf("Select: expected ErrNonInteractive, got %v", err)
	}

	if _, err := MultiSelect("pick many", []Option{{Value: "a", Label: "A"}}, nil); !errors.Is(err, ErrNonInteractive) {
		t.Fatalf("MultiSelect: expected ErrNonInteractive, got %v", err)
	}

	if got, err := Confirm("ok?", true); !errors.Is(err, ErrNonInteractive) || got != true {
		t.Fatalf("Confirm: expected (true, ErrNonInteractive), got (%v, %v)", got, err)
	}
}
