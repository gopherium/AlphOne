// SPDX-License-Identifier: Elastic-2.0

package sdk

import (
	"crypto/sha256"
	"encoding/hex"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// downloadTime is the time every test download was stored at.
var downloadTime = time.Date(2026, time.October, 25, 9, 30, 0, 0, time.UTC)

// shaOf returns the lower case hex SHA-256 of body.
func shaOf(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

// downloadOf returns a download of body under the media type and the filename, its ETag the SHA-256 sha.
func downloadOf(body, mimeType, filename, sha string) Download {
	return Download{
		Content: strings.NewReader(body), ContentType: mimeType, Filename: filename, SHA256: sha, ModTime: downloadTime,
	}
}

// serveDownload answers a GET carrying header with d and returns what ServeDownload wrote.
func serveDownload(t *testing.T, d Download, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/files/receipt", nil)
	for name, values := range header {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	recorder := httptest.NewRecorder()
	ServeDownload(recorder, request, d)
	return recorder
}

func TestServeDownloadSendsTheSafeHeaders(t *testing.T) {
	t.Parallel()

	sha := shaOf("img-bytes")

	recorder := serveDownload(t, downloadOf("img-bytes", "image/jpeg", "", sha), nil)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Body.String(); got != "img-bytes" {
		t.Errorf("body = %q, want %q", got, "img-bytes")
	}
	headers := map[string]string{
		"Content-Type":            "image/jpeg",
		"ETag":                    `"` + sha + `"`,
		"Cache-Control":           "private, max-age=31536000, immutable",
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "sandbox",
		"Content-Disposition":     "inline",
		"Accept-Ranges":           "bytes",
		"Last-Modified":           downloadTime.Format(http.TimeFormat),
	}
	for name, want := range headers {
		if got := recorder.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestServeDownloadServesARange(t *testing.T) {
	t.Parallel()

	recorder := serveDownload(t, downloadOf("range-bytes-payload", "audio/ogg", "", shaOf("range-bytes-payload")),
		http.Header{"Range": {"bytes=6-10"}})

	if recorder.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusPartialContent)
	}
	if got := recorder.Body.String(); got != "bytes" {
		t.Errorf("body = %q, want %q", got, "bytes")
	}
	if got := recorder.Header().Get("Content-Range"); got != "bytes 6-10/19" {
		t.Errorf("Content-Range = %q, want %q", got, "bytes 6-10/19")
	}
}

func TestServeDownloadAnswersAMatchingETagWithNotModified(t *testing.T) {
	t.Parallel()

	sha := shaOf("cached-bytes")

	recorder := serveDownload(t, downloadOf("cached-bytes", "image/png", "", sha),
		http.Header{"If-None-Match": {`"` + sha + `"`}})

	if recorder.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotModified)
	}
	if recorder.Body.Len() != 0 {
		t.Errorf("body length = %d, want 0", recorder.Body.Len())
	}
}

func TestServeDownloadSendsNoETagWithoutAHash(t *testing.T) {
	t.Parallel()

	recorder := serveDownload(t, downloadOf("hashless", "image/png", "", ""), nil)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("ETag"); got != "" {
		t.Errorf("ETag = %q, want none without a hash", got)
	}
}

// dispositionCase is one media type and filename beside the type and disposition a download of them carries.
type dispositionCase struct {
	mimeType        string
	filename        string
	wantType        string
	wantDisposition string
	wantFilename    string
}

// dispositionCases returns every media type and filename the disposition test serves, by the case it covers.
func dispositionCases() map[string]dispositionCase {
	return map[string]dispositionCase{
		"pdf with filename": {mimeType: "application/pdf", filename: "receipt.pdf",
			wantType: "application/pdf", wantDisposition: "attachment", wantFilename: "receipt.pdf"},
		"pdf with unicode filename": {mimeType: "application/pdf", filename: "receipt ö ñ.pdf",
			wantType: "application/pdf", wantDisposition: "attachment", wantFilename: "receipt ö ñ.pdf"},
		"pdf without filename": {mimeType: "application/pdf",
			wantType: "application/pdf", wantDisposition: "attachment"},
		"opus voice note": {mimeType: "audio/ogg; codecs=opus",
			wantType: "audio/ogg; codecs=opus", wantDisposition: "inline"},
		"webp sticker": {mimeType: "image/webp", wantType: "image/webp", wantDisposition: "inline"},
		"mp4 video":    {mimeType: "video/mp4", wantType: "video/mp4", wantDisposition: "inline"},
		"3gpp video":   {mimeType: "video/3gpp", wantType: "video/3gpp", wantDisposition: "inline"},
		"html page": {mimeType: "text/html", filename: "page.html",
			wantType: "text/html", wantDisposition: "attachment", wantFilename: "page.html"},
		"svg image": {mimeType: "image/svg+xml", wantType: "image/svg+xml", wantDisposition: "attachment"},
		"unknown type": {mimeType: "application/x-thing",
			wantType: "application/x-thing", wantDisposition: "attachment"},
		"empty type":      {wantType: "application/octet-stream", wantDisposition: "attachment"},
		"unparsable type": {mimeType: "image/png; =", wantType: "image/png; =", wantDisposition: "attachment"},
	}
}

func TestServeDownloadDispositions(t *testing.T) {
	t.Parallel()

	for testName, tc := range dispositionCases() {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			recorder := serveDownload(t, downloadOf("blob", tc.mimeType, tc.filename, shaOf("blob")), nil)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
			}
			if got := recorder.Header().Get("Content-Type"); got != tc.wantType {
				t.Errorf("Content-Type = %q, want %q", got, tc.wantType)
			}
			disposition, params, err := mime.ParseMediaType(recorder.Header().Get("Content-Disposition"))
			if err != nil {
				t.Fatalf("parsing Content-Disposition %q: %v", recorder.Header().Get("Content-Disposition"), err)
			}
			if disposition != tc.wantDisposition {
				t.Errorf("disposition = %q, want %q", disposition, tc.wantDisposition)
			}
			if params["filename"] != tc.wantFilename {
				t.Errorf("filename = %q, want %q", params["filename"], tc.wantFilename)
			}
		})
	}
}
