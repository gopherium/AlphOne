// SPDX-License-Identifier: Elastic-2.0

package message_test

import (
	"bytes"
	"mime/quotedprintable"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/gopherium/alphone/plugins/email/message"
)

// htmlShowingTheImage is an HTML part that shows the image whose content id is img1@example.com.
var htmlShowingTheImage = htmlPart(`<p><img src="cid:img1@example.com"></p>`)

// imageFile is the header of an image part whose content id is img1@example.com.
const imageFile = "Content-Type: image/png; name=\"wall.png\"\r\nContent-ID: <img1@example.com>"

// inlineCases lists a body part and a file part, and the attachment and inline image names Parse sorts them into.
var inlineCases = []struct {
	name        string
	body        string
	file        string
	attachments []string
	inline      []string
}{
	{
		"a PDF with a content id and an inline disposition",
		"Content-Type: text/plain\r\n\r\nThe quote is attached.",
		"Content-Type: application/pdf; name=\"quote.pdf\"\r\n" +
			"Content-Disposition: inline; filename=\"quote.pdf\"\r\nContent-ID: <pdf1@example.com>",
		[]string{"quote.pdf"}, nil,
	},
	{
		"a PDF with a content id and no disposition",
		"Content-Type: text/plain\r\n\r\nThe quote is attached.",
		"Content-Type: application/pdf; name=\"quote.pdf\"\r\nContent-ID: <pdf1@example.com>",
		[]string{"quote.pdf"}, nil,
	},
	{
		"an image the HTML never shows",
		"Content-Type: text/html\r\n\r\n<p>See the wall.</p>",
		imageFile,
		[]string{"wall.png"}, nil,
	},
	{
		"an image with no content id",
		htmlShowingTheImage,
		"Content-Type: image/png; name=\"wall.png\"\r\nContent-Disposition: inline; filename=\"wall.png\"",
		[]string{"wall.png"}, nil,
	},
	{
		"an image the HTML shows through a cid link",
		htmlShowingTheImage,
		imageFile,
		nil, []string{"wall.png"},
	},
	{
		"an image the HTML shows, sent as an attachment",
		htmlShowingTheImage,
		"Content-Type: image/png; name=\"wall.png\"\r\n" +
			"Content-Disposition: attachment; filename=\"wall.png\"\r\nContent-ID: <img1@example.com>",
		nil, []string{"wall.png"},
	},
	{
		"an image whose content id only starts a longer cid link",
		htmlPart(`<p><img src="cid:img1@example.com.large"></p>`), imageFile, []string{"wall.png"}, nil,
	},
	{
		"an image the HTML names only in its text",
		htmlPart(`<p>The wall is at cid:img1@example.com today.</p>`), imageFile, []string{"wall.png"}, nil,
	},
	{
		"an image the HTML names only in a comment",
		htmlPart(`<!-- <img src="cid:img1@example.com"> -->`), imageFile, []string{"wall.png"}, nil,
	},
	{
		"an image the HTML only links to",
		htmlPart(`<p><a href="cid:img1@example.com">The wall</a></p>`), imageFile, []string{"wall.png"}, nil,
	},
	{
		"an image beside an img element with a web source",
		htmlPart(`<p><img src="https://example.com/wall.png"></p>`), imageFile, []string{"wall.png"}, nil,
	},
	{
		"an image beside an img element with no source",
		htmlPart(`<p><img alt="cid:img1@example.com"></p>`), imageFile, []string{"wall.png"}, nil,
	},
	{
		"an image with no content id beside an empty cid link",
		htmlPart(`<p><img src="cid:"></p>`),
		"Content-Type: image/png; name=\"wall.png\"\r\nContent-Disposition: inline; filename=\"wall.png\"",
		[]string{"wall.png"}, nil,
	},
	{
		"an image whose content id holds a broken escape",
		htmlPart(`<p><img src="cid:img%zz@example.com"></p>`),
		"Content-Type: image/png; name=\"wall.png\"\r\nContent-ID: <img%zz@example.com>",
		nil, []string{"wall.png"},
	},
	{
		"an image the HTML shows through a percent-encoded cid link",
		htmlPart(`<p><img src="cid:img1%40example.com"></p>`), imageFile, nil, []string{"wall.png"},
	},
	{
		"an image the HTML shows through a character reference",
		htmlPart(`<p><img src="cid:img1&#64;example.com"></p>`), imageFile, nil, []string{"wall.png"},
	},
	{
		"an image the HTML shows through an uppercase scheme",
		htmlPart(`<P><IMG SRC="CID:img1@example.com"></P>`), imageFile, nil, []string{"wall.png"},
	},
	{
		"an image whose content id holds a plus sign",
		htmlPart(`<p><img src="cid:wall+1@example.com"></p>`),
		"Content-Type: image/png; name=\"wall.png\"\r\nContent-ID: <wall+1@example.com>",
		nil, []string{"wall.png"},
	},
	{
		"an image whose plus sign the cid link percent-encodes",
		htmlPart(`<p><img src="cid:wall%2B1@example.com"></p>`),
		"Content-Type: image/png; name=\"wall.png\"\r\nContent-ID: <wall+1@example.com>",
		nil, []string{"wall.png"},
	},
	{
		"an image whose plus sign the content id percent-encodes",
		htmlPart(`<p><img src="cid:wall+1@example.com"></p>`),
		"Content-Type: image/png; name=\"wall.png\"\r\nContent-ID: <wall%2B1@example.com>",
		nil, []string{"wall.png"},
	},
	{
		"an image the HTML shows through a cid link with a query",
		htmlPart(`<p><img src="cid:img1@example.com?size=large"></p>`), imageFile, nil, []string{"wall.png"},
	},
	{
		"an image the HTML shows through a cid link with blanks around it",
		htmlPart(`<p><img src=" cid:img1@example.com "></p>`), imageFile, nil, []string{"wall.png"},
	},
	{
		"an image the HTML names through a mailto link",
		htmlPart(`<p><img src="mailto:img1@example.com"></p>`), imageFile, []string{"wall.png"}, nil,
	},
	{
		"an image only an iframe names",
		htmlPart(`<iframe src="cid:img1@example.com"></iframe>`), imageFile, []string{"wall.png"}, nil,
	},
	{
		"an image inside an object element",
		htmlPart(`<object><img src="cid:img1@example.com"></object>`), imageFile, []string{"wall.png"}, nil,
	},
}

func TestParseKeepsTheContentOfEachFile(t *testing.T) {
	t.Parallel()

	cases := []struct {
		file  string
		files func(*message.Message) []message.Attachment
		want  string
	}{
		{"pdf-attachment.eml", attachmentsOf, "%PDF-1.4\n"},
		{"inline-image.eml", inlineImagesOf, "\x89PNG\r\n\x1a\n"},
	}

	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			t.Parallel()

			files := c.files(parseFixture(t, c.file))

			if len(files) != 1 || !bytes.HasPrefix(files[0].Content, []byte(c.want)) {
				t.Errorf("Parse(%s) files = %+v, want one starting %q", c.file, files, c.want)
			}
		})
	}
}

func TestParseSortsInlineImagesFromAttachments(t *testing.T) {
	t.Parallel()

	for _, c := range inlineCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			m, err := message.Parse(strings.NewReader(fileMessage(c.body, c.file)), testLimits)

			if err != nil {
				t.Fatalf("Parse() error = %v, want nil", err)
			}
			if got := fileNames(m.Attachments); !slices.Equal(got, c.attachments) {
				t.Errorf("Parse().Attachments = %q, want %q", got, c.attachments)
			}
			if got := fileNames(m.InlineImages); !slices.Equal(got, c.inline) {
				t.Errorf("Parse().InlineImages = %q, want %q", got, c.inline)
			}
		})
	}
}

func TestParseKeepsAPlusSignInAContentID(t *testing.T) {
	t.Parallel()

	for _, header := range []string{"<wall+1@example.com>", "<wall%2B1@example.com>"} {
		t.Run(header, func(t *testing.T) {
			t.Parallel()

			raw := fileMessage(htmlPart(`<p><img src="cid:wall+1@example.com"></p>`),
				"Content-Type: image/png; name=\"wall.png\"\r\nContent-ID: "+header)

			m, err := message.Parse(strings.NewReader(raw), testLimits)

			if err != nil {
				t.Fatalf("Parse() error = %v, want nil", err)
			}
			if len(m.InlineImages) != 1 || m.InlineImages[0].ContentID != "wall+1@example.com" {
				t.Errorf("Parse().InlineImages = %+v, want one with content id wall+1@example.com", m.InlineImages)
			}
		})
	}
}

func TestParseKeepsEveryByteOfALongQuotedPrintableAttachment(t *testing.T) {
	t.Parallel()

	payload := make([]byte, 5000)
	for i := range payload {
		payload[i] = byte(i)
	}
	var encoded bytes.Buffer
	writer := quotedprintable.NewWriter(&encoded)
	writer.Binary = true
	_, _ = writer.Write(payload)
	_ = writer.Close()
	raw := "Content-Type: multipart/mixed; boundary=\"long\"\r\n\r\n--long\r\n" +
		"Content-Type: application/octet-stream\r\nContent-Disposition: attachment; filename=\"long.bin\"\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n\r\n" + encoded.String() + "\r\n--long--\r\n"

	m, err := message.Parse(strings.NewReader(raw), testLimits)

	if err != nil {
		t.Fatalf("Parse() of a 5000 byte quoted-printable attachment error = %v, want nil", err)
	}
	if len(m.Attachments) != 1 {
		t.Fatalf("Parse() of a 5000 byte quoted-printable attachment gave %d attachments, want 1", len(m.Attachments))
	}
	if !bytes.Equal(m.Attachments[0].Content, payload) {
		t.Errorf("Parse() of a 5000 byte quoted-printable attachment lost bytes")
	}
}

func TestParsePinsAPartWithADispositionAndNoType(t *testing.T) {
	t.Parallel()

	attachments := parseFixture(t, "disposition-without-type.eml").Attachments

	want := message.Attachment{Content: []byte("Bring the colour samples on Friday.\r\n")}
	if len(attachments) != 1 || !reflect.DeepEqual(attachments[0], want) {
		t.Errorf("Parse(disposition-without-type.eml).Attachments = %+v, want only %+v", attachments, want)
	}
}

// attachmentsOf answers the attachments of m.
func attachmentsOf(m *message.Message) []message.Attachment {
	return m.Attachments
}

// inlineImagesOf answers the inline images of m.
func inlineImagesOf(m *message.Message) []message.Attachment {
	return m.InlineImages
}

// fileMessage answers a raw message holding the body part, then one file part with the given header fields.
func fileMessage(body, file string) string {
	return "Content-Type: multipart/related; boundary=\"files\"\r\n\r\n--files\r\n" + body +
		"\r\n--files\r\n" + file + "\r\n\r\nThe file content.\r\n--files--\r\n"
}

// htmlPart answers the header and body of a part holding html.
func htmlPart(html string) string {
	return "Content-Type: text/html\r\n\r\n" + html
}

// fileNames answers the name of each file in files.
func fileNames(files []message.Attachment) []string {
	var names []string
	for _, f := range files {
		names = append(names, f.Name)
	}
	return names
}
