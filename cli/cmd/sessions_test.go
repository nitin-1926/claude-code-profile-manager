package cmd

import "testing"

func TestEncodeCwdForClaude(t *testing.T) {
	cases := map[string]string{
		"/Users/alex/code/repo":     "Users-alex-code-repo",
		"/tmp/with.dots":            "tmp-with-dots",
		"/a//b///c":                 "a-b-c",
		"already-clean":             "already-clean",
		"/":                         "",
		"trailing/slash/":           "trailing-slash",
		"//leading-empty":           "leading-empty",
		"C:\\Users\\alex\\repo":     "C-Users-alex-repo",
		"/Users/a/repo (local)":     "Users-a-repo-local",
	}
	for in, want := range cases {
		if got := encodeCwdForClaude(in); got != want {
			t.Errorf("encodeCwdForClaude(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExtractUserPrompt(t *testing.T) {
	cases := []struct {
		name  string
		entry map[string]interface{}
		want  string
	}{
		{
			name:  "v1 top-level content string",
			entry: map[string]interface{}{"role": "user", "content": "hello"},
			want:  "hello",
		},
		{
			name: "v2 message.content string",
