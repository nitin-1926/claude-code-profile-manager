package cmd

import (
	"reflect"
	"sort"
	"testing"
)

func TestSetNestedKey(t *testing.T) {
	m := map[string]interface{}{}
	setNestedKey(m, "top", "v")
	if m["top"] != "v" {
		t.Errorf("shallow set failed: %v", m)
	}

	setNestedKey(m, "permissions.defaultMode", "acceptEdits")
	perms, ok := m["permissions"].(map[string]interface{})
	if !ok {
		t.Fatal("permissions should be a map")
	}
	if perms["defaultMode"] != "acceptEdits" {
		t.Errorf("nested set failed: %v", perms)
	}

	// Overwriting a scalar with a nested path replaces it.
	m["foo"] = "scalar"
	setNestedKey(m, "foo.bar", "nested")
	foo, ok := m["foo"].(map[string]interface{})
	if !ok {
		t.Fatal("foo should have become a map")
	}
	if foo["bar"] != "nested" {
		t.Errorf("foo.bar = %v", foo)
	}
}

func TestGetNestedKey(t *testing.T) {
	m := map[string]interface{}{
		"top": "v",
		"permissions": map[string]interface{}{
			"defaultMode": "acceptEdits",
			"allow":       []interface{}{"Bash"},
		},
	}
	cases := map[string]interface{}{
		"top":                      "v",
		"permissions.defaultMode":  "acceptEdits",
		"permissions.allow":        []interface{}{"Bash"},
		"permissions.missing":      nil,
		"missing":                  nil,
		"top.inside.a.scalar.path": nil,
	}
	for key, want := range cases {
		got := getNestedKey(m, key)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("getNestedKey(%q) = %v, want %v", key, got, want)
		}
	}
}

func TestDangerousKeysIn(t *testing.T) {
	patch := map[string]interface{}{
		"model":       "safe",
		"hooks":       map[string]interface{}{},
		"permissions": map[string]interface{}{"defaultMode": "bypassPermissions"},
		"extra":       "ok",
	}
	got := dangerousKeysIn(patch)
	sort.Strings(got)
	want := []string{"hooks", "permissions"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("dangerousKeysIn = %v, want %v", got, want)
	}

	if len(dangerousKeysIn(map[string]interface{}{"model": "safe"})) != 0 {
		t.Error("safe patch must return empty slice")
	}
}
