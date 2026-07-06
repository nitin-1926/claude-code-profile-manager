package usage

import (
	"testing"
	"time"
)

func mkEntry(ts string, model string, in int64) entry {
	t, _ := time.Parse(time.RFC3339, ts)
	return entry{ts: t, model: model, tokens: Tokens{Input: in}}
}

// TestGroupBlocksSplitsOn5hGap: two bursts more than 5h apart form two blocks.
func TestGroupBlocksSplitsOn5hGap(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-27T20:00:00Z")
	entries := []entry{
		mkEntry("2026-06-27T09:05:00Z", "opus", 100),
		mkEntry("2026-06-27T09:40:00Z", "opus", 200), // same block (within 5h)
		mkEntry("2026-06-27T18:30:00Z", "sonnet", 300), // >5h after last → new block
	}
	blocks := groupBlocks(entries, now)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].Total != 300 {
		t.Errorf("block 0 total = %d, want 300", blocks[0].Total)
	}
	if blocks[1].Total != 300 {
		t.Errorf("block 1 total = %d, want 300", blocks[1].Total)
	}
	// block start is hour-floored
	if blocks[0].Start != "2026-06-27T09:00:00Z" {
		t.Errorf("block 0 start = %s, want 2026-06-27T09:00:00Z", blocks[0].Start)
	}
}

// TestGroupBlocksActiveBurn: a recent burst yields an active block with burn +
// projection set.
func TestGroupBlocksActiveBurn(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-27T12:00:00Z")
	entries := []entry{
		mkEntry("2026-06-27T10:00:00Z", "opus", 600000), // 60 min before now within block
	}
	blocks := groupBlocks(entries, now)
	if len(blocks) != 1 || !blocks[0].IsActive {
		t.Fatalf("expected 1 active block, got %+v", blocks)
	}
	b := blocks[0]
	if b.BurnTokensPerMin <= 0 {
		t.Errorf("burn rate should be positive, got %f", b.BurnTokensPerMin)
	}
	if b.Cost <= 0 {
		t.Errorf("active block cost should be > 0 (opus tokens), got %f", b.Cost)
	}
	if b.RemainingMinutes <= 0 {
		t.Errorf("remaining minutes should be > 0, got %d", b.RemainingMinutes)
	}
	if b.ProjectedTotal < b.Total {
		t.Errorf("projected total %d should be >= current %d", b.ProjectedTotal, b.Total)
	}
}

// TestGroupBlocksStaleNotActive: an old block (last activity >5h before now) is
// not active.
func TestGroupBlocksStaleNotActive(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-27T23:00:00Z")
	entries := []entry{mkEntry("2026-06-27T09:00:00Z", "opus", 100)}
	blocks := groupBlocks(entries, now)
	if len(blocks) != 1 {
		t.Fatalf("want 1 block, got %d", len(blocks))
	}
	if blocks[0].IsActive {
		t.Error("stale block should not be active")
	}
}
