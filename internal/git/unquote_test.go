package git

import "testing"

func TestUnquoteGitPath(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		// Plain ASCII — no change
		{"README.md", "README.md"},
		// Cyrillic filename encoded as \xNN hex escapes
		{`"\xd0\xb2\xd0\xbb\xd0\xb8\xd1\x8f\xd0\xbd\xd0\xb8\xd0\xb5.txt"`, "влияние.txt"},
		// Mixed ASCII + Cyrillic
		{`"TabbyCard-\xd0\xb2\xd0\xbb\xd0\xb8\xd1\x8f\xd0\xbd\xd0\xb8\xd0\xb5.txt"`, "TabbyCard-влияние.txt"},
		// Standard C escapes
		{`"hello\tworld"`, "hello\tworld"},
		{`"with\\backslash"`, "with\\backslash"},
		{`"with\"quote"`, `with"quote`},
		// Octal escapes (git also uses these)
		{`"\320\262\320\273\320\270\321\217\320\275\320\270\320\265"`, "влияние"},
		// Already unquoted
		{"path/to/file.go", "path/to/file.go"},
		// Empty string
		{"", ""},
	}
	for _, tt := range tests {
		got := unquoteGitPath(tt.in)
		if got != tt.want {
			t.Errorf("unquoteGitPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
