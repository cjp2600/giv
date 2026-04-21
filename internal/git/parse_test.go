package git

import "testing"

func TestParsePorcelainUnicode(t *testing.T) {
	// Git porcelain output with quoted non-ASCII filename
	raw := ` M "TabbyCard-\xd0\xb2\xd0\xbb\xd0\xb8\xd1\x8f\xd0\xbd\xd0\xb8\xd0\xb5.txt"` + "\n"
	files := parsePorcelain(raw)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	want := "TabbyCard-влияние.txt"
	if files[0].Path != want {
		t.Errorf("Path = %q, want %q", files[0].Path, want)
	}
}

func TestParseNameStatusUnicode(t *testing.T) {
	raw := "M\t\"TabbyCard-\\xd0\\xb2\\xd0\\xbb\\xd0\\xb8\\xd1\\x8f\\xd0\\xbd\\xd0\\xb8\\xd0\\xb5.txt\"\n"
	files := parseNameStatus(raw)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	want := "TabbyCard-влияние.txt"
	if files[0].Path != want {
		t.Errorf("Path = %q, want %q", files[0].Path, want)
	}
}
