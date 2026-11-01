// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"net/textproto"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jhillyerd/enmime/v2"
	"github.com/jhillyerd/enmime/v2/mediatype"
)

// ErrLimitsUnset reports a limit of zero or below.
var ErrLimitsUnset = errors.New("message: every limit must be above zero")

// ErrTooLarge reports a raw message, its top header block or a media field over its byte cap.
var ErrTooLarge = errors.New("message: the raw message, its header block or a media field is over its byte cap")

// ErrTooManyParts reports a message with more parts or containers than its limits allow.
var ErrTooManyParts = errors.New("message: the message has more parts than its limits allow")

// ErrMalformed reports a raw message too broken to parse.
var ErrMalformed = errors.New("message: the raw message is malformed")

// Limits bounds what Parse reads.
type Limits struct {
	// MaxBytes is the size of the largest raw message Parse reads.
	MaxBytes int64
	// MaxParts is the most parts Parse reads below the top part, nested containers included.
	MaxParts int
	// MaxContainers is the most Content-Type fields with a boundary the raw message may hold, the top one included.
	MaxContainers int
	// MaxFieldBytes is the longest Content-Type or Content-Disposition field Parse reads, measured unfolded.
	MaxFieldBytes int
	// MaxHeaderBytes is the size of the largest top header block Parse reads, the blank line after it left out.
	MaxHeaderBytes int
	// MaxBodyBytes is the most text and HTML Parse keeps across all body parts, cutting what is over it.
	MaxBodyBytes int
}

// Parse reads one raw message from r within limits.
func Parse(r io.Reader, limits Limits) (*Message, error) {
	if !limits.set() {
		return nil, ErrLimitsUnset
	}
	raw, err := io.ReadAll(io.LimitReader(r, min(limits.MaxBytes, math.MaxInt64-1)+1))
	if err != nil {
		return nil, fmt.Errorf("message: read: %w", err)
	}
	return ParseBytes(raw, limits)
}

// ParseBytes reads the one raw message raw holds within limits, keeping raw as its Raw field.
func ParseBytes(raw []byte, limits Limits) (*Message, error) {
	if err := checkRaw(raw, limits); err != nil {
		return nil, err
	}
	parser := enmime.NewParser(enmime.MaxMIMEParts(limits.MaxParts), enmime.SkipMalformedParts(true))
	root, err := parser.ReadParts(bytes.NewReader(raw))
	var tooMany *enmime.TooManyPartsError
	if errors.As(err, &tooMany) {
		return nil, fmt.Errorf("%w: %w", ErrTooManyParts, err)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	bodyCut := cutBodies(root, limits.MaxBodyBytes)
	envelope, err := parser.EnvelopeFromPart(root)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	m := fromEnvelope(envelope, raw)
	m.BodyCut = bodyCut
	return m, nil
}

// set reports whether every limit is above zero.
func (l Limits) set() bool {
	return l.MaxBytes > 0 && l.MaxParts > 0 && l.MaxContainers > 0 && l.MaxFieldBytes > 0 &&
		l.MaxHeaderBytes > 0 && l.MaxBodyBytes > 0
}

// checkRaw refuses raw when limits are unset or it breaks a limit that can be measured before any parsing.
func checkRaw(raw []byte, limits Limits) error {
	switch {
	case !limits.set():
		return ErrLimitsUnset
	case int64(len(raw)) > limits.MaxBytes:
		return fmt.Errorf("%w: over %d bytes", ErrTooLarge, limits.MaxBytes)
	case len(headerBlock(raw)) > limits.MaxHeaderBytes:
		return fmt.Errorf("%w: a header block over %d bytes", ErrTooLarge, limits.MaxHeaderBytes)
	}
	return checkMediaFields(raw, limits)
}

// headerBlock answers the lines of raw before the first blank line, all of raw when it holds none.
func headerBlock(raw []byte) []byte {
	for start := 0; start < len(raw); {
		line, _, _ := bytes.Cut(raw[start:], []byte("\n"))
		if len(bytes.TrimSuffix(line, []byte("\r"))) == 0 {
			return raw[:start]
		}
		start += len(line) + 1
	}
	return raw
}

// cutBodies cuts the body parts under root to share limit bytes, reporting whether it cut any.
func cutBodies(root *enmime.Part, limit int) bool {
	cut := false
	for _, part := range root.DepthMatchAll(isBody) {
		if len(part.Content) > limit {
			part.Content = cutOnLetter(part.Content, limit)
			cut = true
		}
		limit -= len(part.Content)
	}
	return cut
}

// cutOnLetter answers the start of text up to limit bytes, stepping back to where a letter starts when one is near.
func cutOnLetter(text []byte, limit int) []byte {
	for end := limit; end > max(0, limit-utf8.UTFMax); end-- {
		if utf8.RuneStart(text[end]) {
			return text[:end]
		}
	}
	return text[:limit]
}

// mediaFieldNames lists the fields enmime reads parameters from.
var mediaFieldNames = []string{"Content-Type", "Content-Disposition"}

// checkMediaFields refuses raw when a media field is over the field cap or the containers are over their limit.
func checkMediaFields(raw []byte, limits Limits) error {
	bounds := append(fieldStarts(raw), len(raw))
	containers := 0
	for i := range len(bounds) - 1 {
		name, value := mediaField(raw[bounds[i]:bounds[i+1]])
		if len(value) > limits.MaxFieldBytes {
			return fmt.Errorf("%w: a %s field of %d bytes, over %d", ErrTooLarge, name, len(value), limits.MaxFieldBytes)
		}
		if name == "Content-Type" && declaresBoundary(value) {
			containers++
		}
	}
	if containers > limits.MaxContainers {
		return fmt.Errorf("%w: %d containers, over %d", ErrTooManyParts, containers, limits.MaxContainers)
	}
	return nil
}

// fieldStarts answers where each line of raw that opens a media field starts.
func fieldStarts(raw []byte) []int {
	var starts []int
	for start := 0; start < len(raw); {
		line, _, _ := bytes.Cut(raw[start:], []byte("\n"))
		if opensMediaField(line) {
			starts = append(starts, start)
		}
		start += len(line) + 1
	}
	return starts
}

// opensMediaField reports whether line opens a media field, allowing one blank before the colon.
func opensMediaField(line []byte) bool {
	name, _, found := bytes.Cut(line, []byte(":"))
	name = bytes.TrimSuffix(name, []byte(" "))
	return found && slices.ContainsFunc(mediaFieldNames, func(field string) bool {
		return bytes.EqualFold(name, []byte(field))
	})
}

// mediaField answers the name and the unfolded value of the media field opening fields.
func mediaField(fields []byte) (name, value string) {
	line, _, _ := bytes.Cut(fields, []byte(":"))
	name = textproto.CanonicalMIMEHeaderKey(string(bytes.TrimSuffix(line, []byte(" "))))
	header, _ := enmime.ReadHeader(bufio.NewReader(bytes.NewReader(fields)), ignoredProblems{})
	return name, header.Get(name)
}

// declaresBoundary reports whether the Content-Type value holds a boundary parameter.
func declaresBoundary(value string) bool {
	_, params, _, _ := mediatype.Parse(value)
	return params["boundary"] != ""
}

// ignoredProblems drops what enmime reports about a header it reads.
type ignoredProblems struct{}

// AddError drops a header error.
func (ignoredProblems) AddError(string, string, ...any) {}

// AddWarning drops a header warning.
func (ignoredProblems) AddWarning(string, string, ...any) {}

// fromEnvelope answers the message enmime parsed from raw.
func fromEnvelope(envelope *enmime.Envelope, raw []byte) *Message {
	header := envelope.Root.Header
	m := &Message{
		From:         first(ParseAddresses(header.Get("From"))),
		To:           ParseAddresses(header.Get("To")),
		Cc:           ParseAddresses(header.Get("Cc")),
		Bcc:          ParseAddresses(header.Get("Bcc")),
		ReplyTo:      ParseAddresses(header.Get("Reply-To")),
		Subject:      cleanText(envelope.GetHeader("Subject")),
		MessageID:    first(ParseIDs(header.Get("Message-Id"))),
		InReplyTo:    ParseIDs(strings.Join(header.Values("In-Reply-To"), " ")),
		References:   ParseIDs(strings.Join(header.Values("References"), " ")),
		Text:         cleanText(envelope.Text),
		HTML:         cleanText(envelope.HTML),
		SkippedParts: skippedParts(envelope.Root),
		Raw:          raw,
		root:         envelope.Root,
	}
	m.Date, _ = envelope.Date()
	if index, err := DecodeThreadIndex(header.Get("Thread-Index")); err == nil {
		m.ThreadIndex = &index
	}
	m.Attachments, m.InlineImages = files(envelope, m.HTML)
	return m
}

// skippedParts counts the parts under root that enmime left out as unreadable.
func skippedParts(root *enmime.Part) int {
	skipped := 0
	for _, part := range root.DepthMatchAll(func(*enmime.Part) bool { return true }) {
		for _, problem := range part.Errors {
			if problem.Name == enmime.ErrorMalformedChildPart {
				skipped++
			}
		}
	}
	return skipped
}

// first answers the first item of list, the zero value when list is empty.
func first[T any](list []T) T {
	var zero T
	if len(list) == 0 {
		return zero
	}
	return list[0]
}

// files answers the attachments and the inline images of envelope in message order, given its clean HTML body.
func files(envelope *enmime.Envelope, html string) (attachments, inlineImages []Attachment) {
	listed := make(map[*enmime.Part]bool)
	for _, part := range slices.Concat(envelope.Attachments, envelope.Inlines, envelope.OtherParts) {
		listed[part] = !isBody(part)
	}
	shown := shownContentIDs(html)
	for _, part := range envelope.Root.DepthMatchAll(func(p *enmime.Part) bool { return listed[p] }) {
		file := Attachment{
			Name: cleanText(part.FileName), ContentType: part.ContentType, ContentID: contentID(part), Content: part.Content,
		}
		if strings.HasPrefix(part.ContentType, "image/") && shown[file.ContentID] {
			inlineImages = append(inlineImages, file)
		} else {
			attachments = append(attachments, file)
		}
	}
	return attachments, inlineImages
}

// shownContentIDs answers the content ids of the images Sanitize keeps in document through a cid link.
func shownContentIDs(document string) map[string]bool {
	shown := make(map[string]bool)
	Sanitize(document, func(source *url.URL) {
		if source.Scheme == "cid" && source.Opaque != "" {
			shown[unescaped(source.Opaque)] = true
		}
	})
	return shown
}

// contentID answers the Content-ID of part without its brackets, percent-decoded.
func contentID(part *enmime.Part) string {
	return unescaped(strings.TrimRight(strings.TrimLeft(part.Header.Get("Content-Id"), "<"), ">"))
}

// unescaped answers id percent-decoded and cleaned, leaving a broken escape as it stands.
func unescaped(id string) string {
	if decoded, err := url.PathUnescape(id); err == nil {
		return cleanText(decoded)
	}
	return cleanText(id)
}

// isBody reports whether part is a text or HTML part enmime reads as a body.
func isBody(part *enmime.Part) bool {
	return (part.ContentType == "text/plain" || part.ContentType == "text/html") && part.Disposition != "attachment"
}
