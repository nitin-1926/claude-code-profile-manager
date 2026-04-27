package cmd

import (
	"reflect"
	"testing"
)

func TestFilterRule(t *testing.T) {
	cases := []struct {
		name string
		list []string
		rule string
		want []string
	}{
		{"removes exact match", []string{"Bash(git:*)", "Edit(**/*.go)"}, "Bash(git:*)", []string{"Edit(**/*.go)"}},
		{"no match preserves order", []string{"a", "b", "c"}, "z", []string{"a", "b", "c"}},
		{"removes every occurrence", []string{"x", "x", "y"}, "x", []string{"y"}},
		{"empty list", nil, "x", []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := filterRule(c.list, c.rule)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestListBucket(t *testing.T) {
	cases := []struct {
		name string
		root map[string]interface{}
		in   permissionBucket
		want []string
	}{
		{"interface slice", map[string]interface{}{"allow": []interface{}{"Bash(git:*)", "Edit(**/*.md)"}}, permAllow, []string{"Bash(git:*)", "Edit(**/*.md)"}},
		{"string slice", map[string]interface{}{"allow": []string{"a", "b"}}, permAllow, []string{"a", "b"}},
		{"non-string elements dropped", map[string]interface{}{"allow": []interface{}{"a", 42, "b"}}, permAllow, []string{"a", "b"}},
		{"missing key", map[string]interface{}{}, permAllow, nil},
		{"wrong type", map[string]interface{}{"allow": "not-a-list"}, permAllow, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := listBucket(c.root, c.in)
			if len(got) != len(c.want) {
				t.Fatalf("len mismatch: got %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("at %d: got %q, want %q", i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestPruneEmptyBuckets(t *testing.T) {
	t.Run("strips empty []string bucket", func(t *testing.T) {
		root := map[string]interface{}{"allow": []string{}, "ask": []string{"x"}}
		pruneEmptyBuckets(root)
		if _, ok := root["allow"]; ok {
			t.Error("empty allow should be removed")
		}
		if _, ok := root["ask"]; !ok {
			t.Error("non-empty ask must stay")
		}
	})
	t.Run("strips empty []interface{} bucket", func(t *testing.T) {
		// Pre-refactor this path silently kept the empty array because the
		// type switch only matched []string. Test that the new behavior
		// covers the post-LoadJSON case too.
		root := map[string]interface{}{"deny": []interface{}{}}
		pruneEmptyBuckets(root)
		if _, ok := root["deny"]; ok {
			t.Error("empty []interface{} deny must be removed")
		}
	})
	t.Run("preserves non-permission keys", func(t *testing.T) {
		root := map[string]interface{}{"allow": []string{}, "defaultMode": "acceptEdits"}
		pruneEmptyBuckets(root)
		if root["defaultMode"] != "acceptEdits" {
			t.Error("defaultMode must survive prune")
		}
	})
}
