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
