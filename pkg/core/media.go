package core

import (
	"fmt"
	"path/filepath"
	"strings"
)

// mediaMIMETypes maps a lowercase file extension (without the dot) to its MIME
// type. This is the allowlist of media the `addMedia` command accepts across
// drivers — mirroring the set real devices' Photos / MediaStore pickers index.
var mediaMIMETypes = map[string]string{
	"jpg":  "image/jpeg",
	"jpeg": "image/jpeg",
	"png":  "image/png",
	"gif":  "image/gif",
	"webp": "image/webp",
	"heic": "image/heic",
	"heif": "image/heif",
	"bmp":  "image/bmp",
	"mp4":  "video/mp4",
	"mov":  "video/quicktime",
	"m4v":  "video/x-m4v",
}

// documentMIMETypes is the second allowlist `addMedia` accepts: files an app
// picks through the system document picker rather than the photo picker.
// They live in a different place on every platform (Android's Downloads,
// the simulator's "On My iPhone" storage) and nowhere reachable on a physical
// iPhone, so the two sets are kept apart. Requested in #167.
var documentMIMETypes = map[string]string{
	"pdf":  "application/pdf",
	"doc":  "application/msword",
	"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"xls":  "application/vnd.ms-excel",
	"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"ppt":  "application/vnd.ms-powerpoint",
	"pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	"txt":  "text/plain",
	"csv":  "text/csv",
	"rtf":  "application/rtf",
	"json": "application/json",
	"zip":  "application/zip",
}

// SupportedMediaExtensions is the human-readable allowlist for error messages.
const SupportedMediaExtensions = "images jpg, jpeg, png, gif, webp, heic, heif, bmp; videos mp4, mov, m4v; documents pdf, doc, docx, xls, xlsx, ppt, pptx, txt, csv, rtf, json, zip"

// MediaMIMEType returns the MIME type for a media or document file path based
// on its extension, and whether the extension is supported.
func MediaMIMEType(path string) (string, bool) {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if mime, ok := mediaMIMETypes[ext]; ok {
		return mime, true
	}
	mime, ok := documentMIMETypes[ext]
	return mime, ok
}

// IsDocumentMedia reports whether the path is a document rather than a photo
// or video — the kind the system file picker lists, not the photo picker.
func IsDocumentMedia(path string) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	_, ok := documentMIMETypes[ext]
	return ok
}

// SplitMediaDocuments partitions paths into photos/videos and documents,
// preserving order, so a driver can route each to where its picker looks.
func SplitMediaDocuments(paths []string) (media, documents []string) {
	for _, p := range paths {
		if IsDocumentMedia(p) {
			documents = append(documents, p)
		} else {
			media = append(media, p)
		}
	}
	return media, documents
}

// IsVideoMedia reports whether the path's extension is a supported video type.
func IsVideoMedia(path string) bool {
	mime, ok := MediaMIMEType(path)
	return ok && strings.HasPrefix(mime, "video/")
}

// ValidateMediaFiles checks that every path has a supported media extension,
// returning an error naming the first unsupported file. An empty list is an
// error (nothing to add).
func ValidateMediaFiles(paths []string) error {
	if len(paths) == 0 {
		return fmt.Errorf("no media files specified")
	}
	for _, p := range paths {
		if _, ok := MediaMIMEType(p); !ok {
			return fmt.Errorf("unsupported media type for %q (supported: %s)", p, SupportedMediaExtensions)
		}
	}
	return nil
}

// AndroidDocumentDir is where addMedia places documents on Android. The system
// file picker's Downloads root lists this directory straight from disk, so a
// pushed file is selectable without any MediaStore registration — unlike
// photos and videos, which the photo picker only sees once indexed.
const AndroidDocumentDir = "/sdcard/Download"

// AndroidFilePusher is the slice of an Android device addMedia needs to place
// a document: a shell for mkdir and the scan, and adb push.
type AndroidFilePusher interface {
	Shell(cmd string) (string, error)
	Push(local, remote string) error
}

// PushAndroidDocument copies a local document into AndroidDocumentDir and
// asks MediaStore to index it, so both the file picker (disk) and apps that
// query the Downloads collection (index) can find it. Returns the remote path.
func PushAndroidDocument(dev AndroidFilePusher, local string) (string, error) {
	if _, err := dev.Shell("mkdir -p " + ShellQuote(AndroidDocumentDir)); err != nil {
		return "", fmt.Errorf("create %s: %w", AndroidDocumentDir, err)
	}
	remote := AndroidDocumentDir + "/" + filepath.Base(local)
	if err := dev.Push(local, remote); err != nil {
		return "", fmt.Errorf("push %s: %w", filepath.Base(local), err)
	}
	// Best effort: the picker does not need it, MediaStore.Downloads queries do.
	scan := fmt.Sprintf("content call --uri content://media --method scan_file --arg %s", ShellQuote(remote))
	_, _ = dev.Shell(scan)
	return remote, nil
}
