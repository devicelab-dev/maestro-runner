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

// MediaMIMEType returns the MIME type for a media file path based on its
// extension, and whether the extension is supported.
func MediaMIMEType(path string) (string, bool) {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	mime, ok := mediaMIMETypes[ext]
	return mime, ok
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
			return fmt.Errorf("unsupported media type for %q (supported: jpg, jpeg, png, gif, webp, heic, heif, bmp, mp4, mov, m4v)", p)
		}
	}
	return nil
}
