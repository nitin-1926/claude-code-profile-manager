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
