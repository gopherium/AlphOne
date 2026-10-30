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
	"slices"
	"strings"

	"github.com/jhillyerd/enmime/v2"
	"github.com/jhillyerd/enmime/v2/mediatype"
)

// ErrLimitsUnset reports a limit of zero or below.
var ErrLimitsUnset = errors.New("message: every limit must be above zero")

// ErrTooLarge reports a raw message over the byte cap, or a media field over the field cap.
var ErrTooLarge = errors.New("message: the raw message or a media field is over its byte cap")

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
	if int64(len(raw)) > limits.MaxBytes {
		return nil, fmt.Errorf("%w: over %d bytes", ErrTooLarge, limits.MaxBytes)
	}
	if err := checkMediaFields(raw, limits); err != nil {
		return nil, err
	}
	envelope, err := enmime.NewParser(enmime.MaxMIMEParts(limits.MaxParts)).ReadEnvelope(bytes.NewReader(raw))
	var tooMany *enmime.TooManyPartsError
	if errors.As(err, &tooMany) {
		return nil, fmt.Errorf("%w: %w", ErrTooManyParts, err)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	return fromEnvelope(envelope, raw), nil
}

// set reports whether every limit is above zero.
func (l Limits) set() bool {
	return l.MaxBytes > 0 && l.MaxParts > 0 && l.MaxContainers > 0 && l.MaxFieldBytes > 0
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

// mediaField answers the name and the unfolded value of the media field opening fields, read as enmime reads a header.
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
		From:       first(addresses(envelope, "From")),
		To:         addresses(envelope, "To"),
		Cc:         addresses(envelope, "Cc"),
		ReplyTo:    addresses(envelope, "Reply-To"),
		Subject:    envelope.GetHeader("Subject"),
		MessageID:  first(ParseIDs(header.Get("Message-Id"))),
		InReplyTo:  ParseIDs(strings.Join(header.Values("In-Reply-To"), " ")),
		References: ParseIDs(strings.Join(header.Values("References"), " ")),
		Text:       envelope.Text,
		HTML:       envelope.HTML,
		Raw:        raw,
		root:       envelope.Root,
	}
	m.Date, _ = envelope.Date()
	if index, err := DecodeThreadIndex(header.Get("Thread-Index")); err == nil {
		m.ThreadIndex = &index
	}
	m.Attachments, m.InlineImages = files(envelope)
	return m
}

// addresses answers the normalized mailboxes the address field key names.
func addresses(envelope *enmime.Envelope, key string) []Address {
	list, _ := envelope.AddressList(key)
	var kept []Address
	for _, a := range list {
		if address, ok := NormalizeAddress(*a); ok {
			kept = append(kept, address)
		}
	}
	return kept
}

// first answers the first item of list, the zero value when list is empty.
func first[T any](list []T) T {
	var zero T
	if len(list) == 0 {
		return zero
	}
	return list[0]
}

// files answers the attachments and the inline images of envelope in message order.
func files(envelope *enmime.Envelope) (attachments, inlineImages []Attachment) {
	listed := make(map[*enmime.Part]bool)
	for _, part := range slices.Concat(envelope.Attachments, envelope.Inlines, envelope.OtherParts) {
		listed[part] = !isBody(part)
	}
	for _, part := range envelope.Root.DepthMatchAll(func(p *enmime.Part) bool { return listed[p] }) {
		file := Attachment{
			Name: part.FileName, ContentType: part.ContentType, ContentID: part.ContentID, Content: part.Content,
		}
		if showsInline(part, envelope.HTML) {
			inlineImages = append(inlineImages, file)
		} else {
			attachments = append(attachments, file)
		}
	}
	return attachments, inlineImages
}

// showsInline reports whether part is an image html shows through a cid link.
func showsInline(part *enmime.Part, html string) bool {
	return strings.HasPrefix(part.ContentType, "image/") && part.ContentID != "" &&
		strings.Contains(html, "cid:"+part.ContentID)
}

// isBody reports whether part is a text or HTML part enmime reads as a body.
func isBody(part *enmime.Part) bool {
	return (part.ContentType == "text/plain" || part.ContentType == "text/html") && part.Disposition != "attachment"
}
