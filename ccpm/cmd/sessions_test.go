package cmd

import "testing"

// These expectations previously encoded a bug: runs of non-alphanumerics were
// collapsed and the ends trimmed, so "/Users/alex/code/repo" came out as
// "Users-alex-code-repo" — a directory name that exists nowhere. Claude Code
// writes one dash per non-alphanumeric character and trims nothing, so the real
// directory is "-Users-alex-code-repo". Verified against 18 real project
// directories by usage.TestEncodeCwdMatchesRealLayout.
func TestEncodeCwdForClaude(t *testing.T) {
	cases := map[string]string{
		"/Users/alex/code/repo": "-Users-alex-code-repo",
		"/tmp/with.dots":        "-tmp-with-dots",
		"/a//b///c":             "-a--b---c",
		"already-clean":         "already-clean",
		"/":                     "-",
		"trailing/slash/":       "trailing-slash-",
		"//leading-empty":       "--leading-empty",
		"C:\\Users\\alex\\repo": "C--Users-alex-repo",
		"/Users/a/repo (local)": "-Users-a-repo--local-",
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
			entry: map[string]interface{}{
				"message": map[string]interface{}{"role": "user", "content": "hi there"},
			},
			want: "hi there",
		},
		{
			name: "v2 message.content typed blocks",
			entry: map[string]interface{}{
				"message": map[string]interface{}{
					"role": "user",
					"content": []interface{}{
						map[string]interface{}{"type": "image"},
						map[string]interface{}{"type": "text", "text": "   question   "},
					},
				},
			},
			want: "question",
		},
		{
			name:  "assistant role returns empty",
			entry: map[string]interface{}{"role": "assistant", "content": "nope"},
			want:  "",
		},
		{
			name:  "no known shape",
			entry: map[string]interface{}{"foo": "bar"},
			want:  "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := extractUserPrompt(c.entry); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
