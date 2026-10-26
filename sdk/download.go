// SPDX-License-Identifier: Elastic-2.0

package sdk

import (
	"io"
	"mime"
	"net/http"
	"strings"
	"time"
)

// octetStream is the media type a download without one carries.
const octetStream = "application/octet-stream"

// Download is one stored file a plugin serves to a browser.
type Download struct {
	// Content is the file the download reads.
	Content io.ReadSeeker
	// ContentType is the media type the download carries, application/octet-stream when empty.
	ContentType string
	// Filename is the name a browser saves an attachment under, none when empty.
	Filename string
	// SHA256 is the hex SHA-256 the ETag carries, no ETag when empty.
	SHA256 string
	// ModTime is the time Last-Modified carries.
	ModTime time.Time
}

// ServeDownload writes d to w as a download a browser never runs, answering ranges and conditional requests.
func ServeDownload(w http.ResponseWriter, r *http.Request, d Download) {
	mimeType := d.ContentType
	if mimeType == "" {
		mimeType = octetStream
	}
	header := w.Header()
	header.Set("Content-Type", mimeType)
	header.Set("Cache-Control", "private, max-age=31536000, immutable")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Content-Security-Policy", "sandbox")
	header.Set("Content-Disposition", mediaDisposition(mimeType, d.Filename))
	if d.SHA256 != "" {
		header.Set("ETag", `"`+d.SHA256+`"`)
	}
	http.ServeContent(w, r, "", d.ModTime, d.Content)
}

// mediaDisposition returns the Content-Disposition of the media type, inline only for the safe allowlist.
func mediaDisposition(mimeType, filename string) string {
	if renderableInline(mimeType) {
		return "inline"
	}
	if filename == "" {
		return "attachment"
	}
	return mime.FormatMediaType("attachment", map[string]string{"filename": filename})
}

// renderableInline reports whether a browser may show the media type inline.
func renderableInline(mimeType string) bool {
	mediaType, _, err := mime.ParseMediaType(mimeType)
	if err != nil {
		return false
	}
	if strings.HasPrefix(mediaType, "audio/") {
		return true
	}
	switch mediaType {
	case "image/jpeg", "image/png", "image/webp", "video/mp4", "video/3gpp":
		return true
	}
	return false
}
