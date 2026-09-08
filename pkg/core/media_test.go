package core

import (
	"strings"
	"testing"
)

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
		{"e.txt", "text/plain", true}, // documents are accepted since #167
		{"e.exe", "", false},
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

func TestMediaMIMEType_Documents(t *testing.T) {
	for path, want := range map[string]string{
		"report.pdf": "application/pdf", "Letter.DOCX": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"data.csv": "text/csv", "notes.txt": "text/plain",
	} {
		mime, ok := MediaMIMEType(path)
		if !ok || mime != want {
			t.Errorf("MediaMIMEType(%q) = (%q,%v), want (%q,true)", path, mime, ok, want)
		}
		if !IsDocumentMedia(path) {
			t.Errorf("IsDocumentMedia(%q) = false", path)
		}
	}
	if IsDocumentMedia("a.jpg") || IsDocumentMedia("b.mp4") {
		t.Error("photos and videos are not documents")
	}
	if err := ValidateMediaFiles([]string{"a.pdf", "b.jpg"}); err != nil {
		t.Errorf("a pdf should validate: %v", err)
	}
	if err := ValidateMediaFiles([]string{"a.exe"}); err == nil {
		t.Error("an unknown extension should still be rejected")
	}
}

func TestSplitMediaDocuments(t *testing.T) {
	media, docs := SplitMediaDocuments([]string{"a.jpg", "b.pdf", "c.mp4", "d.docx"})
	if len(media) != 2 || media[0] != "a.jpg" || media[1] != "c.mp4" {
		t.Errorf("media = %v", media)
	}
	if len(docs) != 2 || docs[0] != "b.pdf" || docs[1] != "d.docx" {
		t.Errorf("documents = %v", docs)
	}
}

type fakePusher struct {
	cmds   []string
	pushes [][2]string
}

func (f *fakePusher) Shell(cmd string) (string, error) { f.cmds = append(f.cmds, cmd); return "", nil }
func (f *fakePusher) Push(l, r string) error {
	f.pushes = append(f.pushes, [2]string{l, r})
	return nil
}

func TestPushAndroidDocument(t *testing.T) {
	dev := &fakePusher{}
	remote, err := PushAndroidDocument(dev, "/tmp/some dir/report.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if remote != "/sdcard/Download/report.pdf" {
		t.Errorf("remote = %q, want the Downloads root the file picker lists", remote)
	}
	if len(dev.pushes) != 1 || dev.pushes[0][1] != remote {
		t.Errorf("pushes = %v", dev.pushes)
	}
	if len(dev.cmds) != 2 || !strings.HasPrefix(dev.cmds[0], "mkdir -p ") || !strings.Contains(dev.cmds[0], "/sdcard/Download") || !strings.Contains(dev.cmds[1], "scan_file") {
		t.Errorf("shell commands = %v", dev.cmds)
	}
}
