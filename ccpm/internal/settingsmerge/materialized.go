package settingsmerge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/atomicwrite"
)

// materializedFile is the per-profile sidecar recording what ccpm itself wrote
// into <profileDir>/settings.json and .claude.json#mcpServers on the last
// MaterializeAll. Those files are also written by Claude Code and the user,
// and each rebuild starts from them — without this record ccpm cannot tell a
// key it wrote (and must drop once its source is gone) from one the user
// wrote (and must keep), so everything it ever wrote lived forever.
const materializedFile = ".ccpm-materialized.json"

// materialized holds fingerprints, never values: Settings mirrors the shape
// of what ccpm wrote with each leaf replaced by fingerprint(leaf), and
// MCPServers maps each server name to fingerprint(definition). The record
// only has to answer "is this still exactly what ccpm wrote?", and env maps
// and MCP definitions carry tokens that must not be copied into a file that
// rides along in `ccpm export` (which leaves .claude.json out for that reason).
type materialized struct {
	Settings   map[string]interface{} `json:"settings"`
	MCPServers map[string]interface{} `json:"mcpServers"`
}

func fingerprint(v interface{}) string {
	b, _ := json.Marshal(v) // map keys marshal sorted, so this is canonical
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// fingerprintLeaves returns m's shape with every leaf (scalar, array, or empty
// object) replaced by its fingerprint.
func fingerprintLeaves(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		if vm, ok := v.(map[string]interface{}); ok && len(vm) > 0 {
			out[k] = fingerprintLeaves(vm)
			continue
		}
		out[k] = fingerprint(v)
	}
	return out
}

// loadMaterialized reads the sidecar. Missing or unreadable yields an empty
// record, which treats nothing in the profile files as ccpm's: a profile
// upgraded from a ccpm without the sidecar (or with a damaged one) keeps all
// its data rather than losing any.
func loadMaterialized(profileDir string) materialized {
	var m materialized
	data, err := atomicwrite.ReadFile(filepath.Join(profileDir, materializedFile))
	if err != nil || json.Unmarshal(data, &m) != nil {
		return materialized{}
	}
	return m
}

// userOwned returns the part of existing (a profile file's current contents)
// that ccpm did not write: leaves still matching the fingerprint ccpm
// recorded in prev (see fingerprintLeaves) are dropped; leaves ccpm never wrote, or that changed since, are kept.
// cur is what ccpm contributes now. An object the user edited is split
// per-leaf only while ccpm still provides it; once ccpm's source for it is
// gone the user's copy is kept whole, so a record like statusLine never ends
// up with only the fields the user touched.
func userOwned(existing, prev, cur map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(existing))
	for k, ev := range existing {
		pv, wrote := prev[k]
		if !wrote {
			out[k] = cloneJSONValue(ev)
			continue
		}
		em, eIsMap := ev.(map[string]interface{})
		pm, pIsMap := pv.(map[string]interface{})
		if eIsMap && pIsMap && len(em) > 0 {
			cm, curHas := cur[k].(map[string]interface{})
			switch sub := userOwned(em, pm, cm); {
			case len(sub) == 0:
				// every leaf is ccpm's
			case !curHas:
				out[k] = cloneJSONValue(ev)
			default:
				out[k] = sub
			}
			continue
		}
		if pv != fingerprint(ev) {
			out[k] = cloneJSONValue(ev)
		}
	}
	return out
}

// stripEqual returns a minus every leaf whose value b holds identically.
// Used to record only what ccpm actually contributed: a leaf the user's own
// layer already holds with the same value stays the user's.
func stripEqual(a, b map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(a))
	for k, av := range a {
		bv, ok := b[k]
		if !ok {
			out[k] = cloneJSONValue(av)
			continue
		}
		am, aIsMap := av.(map[string]interface{})
		bm, bIsMap := bv.(map[string]interface{})
		if aIsMap && bIsMap && len(am) > 0 {
			if sub := stripEqual(am, bm); len(sub) > 0 {
				out[k] = sub
			}
			continue
		}
		if !equalJSON(av, bv) {
			out[k] = cloneJSONValue(av)
		}
	}
	return out
}
