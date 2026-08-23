package core

import "testing"

func TestMediaMIMEType(t *testing.T) {
	cases := []struct {
		path string
		mime string
		ok   bool
	}{
		{"a.jpg", "image/jpeg", true},
		{"a.JPEG", "image/jpeg", true},
		{"dir/b.png", "image/png", true},
		{"c.mp4", "video/mp4", true},
		{"d.mov", "video/quicktime", true},
		{"e.txt", "", false},
		{"noext", "", false},
	}
	for _, c := range cases {
		mime, ok := MediaMIMEType(c.path)
		if ok != c.ok || mime != c.mime {
			t.Errorf("MediaMIMEType(%q) = (%q,%v), want (%q,%v)", c.path, mime, ok, c.mime, c.ok)
		}
	}
}

func TestIsVideoMedia(t *testing.T) {
	if !IsVideoMedia("x.mp4") {
		t.Error("mp4 should be video")
	}
	if IsVideoMedia("x.png") {
		t.Error("png should not be video")
	}
}

func TestValidateMediaFiles(t *testing.T) {
	if err := ValidateMediaFiles(nil); err == nil {
		t.Error("empty list should error")
	}
	if err := ValidateMediaFiles([]string{"a.jpg", "b.mp4"}); err != nil {
		t.Errorf("valid files errored: %v", err)
	}
	if err := ValidateMediaFiles([]string{"a.jpg", "b.exe"}); err == nil {
		t.Error("unsupported file should error")
	}
}
