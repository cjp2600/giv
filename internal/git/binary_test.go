package git

import "testing"

func TestIsBinaryPath(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"image.png", true},
		{"photo.JPEG", true},
		{"archive.tar.gz", true},
		{"program.exe", true},
		{"doc.pdf", true},
		{"font.woff2", true},
		{"data.sqlite3", true},
		{"code.go", false},
		{"readme.md", false},
		{"config.json", false},
		{"style.css", false},
		{"Makefile", false},
		{"noext", false},
		{"icon.svg", false}, // SVG is text
	}
	for _, tc := range cases {
		if got := IsBinaryPath(tc.name); got != tc.want {
			t.Errorf("IsBinaryPath(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestIsBinaryContent(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if IsBinaryContent(nil) {
			t.Error("nil should not be binary")
		}
		if IsBinaryContent([]byte{}) {
			t.Error("empty should not be binary")
		}
	})

	t.Run("plain text", func(t *testing.T) {
		if IsBinaryContent([]byte("hello world\nline two\n")) {
			t.Error("plain text detected as binary")
		}
	})

	t.Run("NUL byte", func(t *testing.T) {
		data := []byte("hello\x00world")
		if !IsBinaryContent(data) {
			t.Error("data with NUL byte should be binary")
		}
	})

	t.Run("NUL beyond sniff window", func(t *testing.T) {
		data := make([]byte, binarySniffSize+100)
		for i := range data {
			data[i] = 'A'
		}
		data[binarySniffSize+50] = 0x00
		if IsBinaryContent(data) {
			t.Error("NUL beyond sniff window should not trigger binary")
		}
	})

	t.Run("ELF magic", func(t *testing.T) {
		data := []byte{0x7F, 'E', 'L', 'F', 1, 1, 1, 0}
		if !IsBinaryContent(data) {
			t.Error("ELF header should be binary")
		}
	})

	t.Run("PDF magic", func(t *testing.T) {
		data := []byte("%PDF-1.4 rest of content")
		if !IsBinaryContent(data) {
			t.Error("PDF header should be binary")
		}
	})

	t.Run("gzip magic", func(t *testing.T) {
		data := []byte{0x1F, 0x8B, 0x08, 0x00}
		if !IsBinaryContent(data) {
			t.Error("gzip header should be binary")
		}
	})

	t.Run("MZ/PE magic", func(t *testing.T) {
		data := []byte{'M', 'Z', 0x90, 0x00}
		if !IsBinaryContent(data) {
			t.Error("MZ header should be binary")
		}
	})

	t.Run("ZIP/PK magic", func(t *testing.T) {
		data := []byte{'P', 'K', 0x03, 0x04, 0x14, 0x00}
		if !IsBinaryContent(data) {
			t.Error("PK/ZIP header should be binary")
		}
	})

	t.Run("UTF-8 with BOM", func(t *testing.T) {
		data := append([]byte{0xEF, 0xBB, 0xBF}, []byte("hello")...)
		if IsBinaryContent(data) {
			t.Error("UTF-8 BOM text should not be binary")
		}
	})
}
