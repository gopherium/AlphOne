// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/vikstrous/dataloadgen"

	"github.com/gopherium/alphone/sdk"
)

// errMailNotHeld reports a mail the workspace of the request holds no row for.
var errMailNotHeld = errors.New("email: the workspace holds no such mail")

// errTooManyBodies reports a request that asks for more mail bodies than it may read.
var errTooManyBodies = errors.New("email: the request asks for more mail bodies than it may read")

// errTooManyLists reports a request that asks for more people and file lists than it may read.
var errTooManyLists = errors.New("email: the request asks for more people and file lists than it may read")

// mailBody is the text and the safe HTML one mail keeps.
type mailBody struct {
	// text is the plain text the mail keeps.
	text string
	// html is the HTML the mail keeps, safe to show.
	html string
}

// attachmentView is one file of a mail as the graph answers it.
type attachmentView struct {
	// id names the file's row.
	id uuid.UUID
	// position is the place of the file among the files of its mail.
	position int
	// filename is the name the mail gives the file.
	filename string
	// contentType is the media type the file is kept under.
	contentType string
	// size is the size of the file.
	size int64
	// inline reports an image the HTML body shows.
	inline bool
	// contentID is the Content-ID of the file, empty for none.
	contentID string
	// state says whether the file is kept, and why not when it is not.
	state string
}

// mailLoaders reads the people, bodies, files and contact ids of the mail one graph request asks for in batches.
type mailLoaders struct {
	// db reads the mail of every workspace.
	db querier
	// bodiesPerRequest is the most mail bodies one request reads.
	bodiesPerRequest int
	// listsPerRequest is the most people and file lists one request reads.
	listsPerRequest int
	// batchWait is how long a loader gathers keys before it reads them, a millisecond when zero.
	batchWait time.Duration
}

// readBound counts the reads of one kind a graph request asks for against the most it may ask for.
type readBound struct {
	// asked counts the reads the request asked for.
	asked atomic.Int64
	// most is the most reads the request may ask for.
	most int
	// reason names the bound in the rejection of a read past it.
	reason string
	// err is the error the rejection of a read past the bound wraps.
	err error
}

// admit counts one more read, rejecting it once the count passes the bound.
func (b *readBound) admit() error {
	if b.asked.Add(1) > int64(b.most) {
		return sdk.GraphError{Code: "VALIDATION", Reason: b.reason, Meta: map[string]any{"max": b.most}, Err: b.err}
	}
	return nil
}

// boundedLoader is a loader of one graph request beside the bound that admits each read asked of it.
type boundedLoader[T any] struct {
	// loader reads the values of many mails in one statement.
	loader *dataloadgen.Loader[uuid.UUID, T]
	// bound admits or rejects each read asked of the loader.
	bound *readBound
}

// bounded returns loader with each read asked of it admitted by bound.
func bounded[T any](loader *dataloadgen.Loader[uuid.UUID, T], bound *readBound) boundedLoader[T] {
	return boundedLoader[T]{loader: loader, bound: bound}
}

// load returns the value of the mail id once the bound admits the read, never reading a rejected one.
func (b boundedLoader[T]) load(ctx context.Context, id uuid.UUID) (T, error) {
	if err := b.bound.admit(); err != nil {
		var none T
		return none, err
	}
	return b.loader.Load(ctx, id)
}

// requestLoaders are the loaders of one graph request, the bounded ones beside the bounds that admit their reads.
type requestLoaders struct {
	// participants loads the people each mail names, within the bound of lists.
	participants boundedLoader[[]participant]
	// bodies loads the text and safe HTML of each mail, within the bound of bodies.
	bodies boundedLoader[mailBody]
	// attachments loads the files of each mail in position order, within the bound of lists.
	attachments boundedLoader[[]attachmentView]
	// contactIDs loads the contacts each mail is linked to.
	contactIDs *dataloadgen.Loader[uuid.UUID, []uuid.UUID]
}

// requestLoadersKey keys the loaders of a graph request in its request scope.
type requestLoadersKey struct{}

// forRequest returns the loaders of the request ctx carries, building them once per request.
func (m mailLoaders) forRequest(ctx context.Context) (*requestLoaders, error) {
	return sdk.ScopedValue(ctx, requestLoadersKey{}, func() *requestLoaders {
		wait := dataloadgen.WithWait(cmp.Or(m.batchWait, time.Millisecond))
		bodies := &readBound{most: m.bodiesPerRequest, reason: "bodies_per_request_exceeded", err: errTooManyBodies}
		lists := &readBound{most: m.listsPerRequest, reason: "lists_per_request_exceeded", err: errTooManyLists}
		return &requestLoaders{
			participants: bounded(dataloadgen.NewLoader(m.fetchParticipants, wait), lists),
			bodies:       bounded(dataloadgen.NewLoader(m.fetchBodies, wait), bodies),
			attachments:  bounded(dataloadgen.NewLoader(m.fetchAttachments, wait), lists),
			contactIDs:   dataloadgen.NewLoader(m.fetchContactIDs, wait),
		}
	})
}

// participants returns the people the mail id names, rejecting each list the request asks for past its bound.
func (m mailLoaders) participants(ctx context.Context, id uuid.UUID) ([]participant, error) {
	loaders, err := m.forRequest(ctx)
	if err != nil {
		return nil, err
	}
	return loaders.participants.load(ctx, id)
}

// body returns the text and safe HTML of the mail id, rejecting each body the request asks for past its bound.
func (m mailLoaders) body(ctx context.Context, id uuid.UUID) (mailBody, error) {
	loaders, err := m.forRequest(ctx)
	if err != nil {
		return mailBody{}, err
	}
	return loaders.bodies.load(ctx, id)
}

// attachments returns the files of the mail id in position order, rejecting each list past the bound of the request.
func (m mailLoaders) attachments(ctx context.Context, id uuid.UUID) ([]attachmentView, error) {
	loaders, err := m.forRequest(ctx)
	if err != nil {
		return nil, err
	}
	return loaders.attachments.load(ctx, id)
}

// forgetFiles drops the files of the mail id from the loaders of the request ctx.
func (m mailLoaders) forgetFiles(ctx context.Context, id uuid.UUID) {
	if loaders, err := m.forRequest(ctx); err == nil {
		loaders.attachments.loader.Clear(id)
	}
}

// contactIDs returns the contacts the mail id is linked to, through the loader of the request.
func (m mailLoaders) contactIDs(ctx context.Context, id uuid.UUID) ([]uuid.UUID, error) {
	loaders, err := m.forRequest(ctx)
	if err != nil {
		return nil, err
	}
	return loaders.contactIDs.Load(ctx, id)
}

// loaded is one row a loader reads, beside the mail it answers for.
type loaded[T any] struct {
	// mail names the mail the row answers for.
	mail uuid.UUID
	// value is what the row holds.
	value T
}

// whole returns value as the one field a row scans.
func whole[T any](value *T) []any {
	return []any{value}
}

// fields returns the fields of the body a row scans, in the order of its columns.
func (b *mailBody) fields() []any {
	return []any{&b.text, &b.html}
}

// fields returns the fields of the file a row scans, in the order of its columns.
func (f *attachmentView) fields() []any {
	return []any{&f.id, &f.position, &f.filename, &f.contentType, &f.size, &f.inline, &f.contentID, &f.state}
}

// readLoaded reads statement over the workspace ctx serves and the mails ids, its failure naming what it loads.
func readLoaded[T any](
	ctx context.Context, db querier, what, statement string, ids []uuid.UUID, fields func(*T) []any,
) ([]loaded[T], error) {
	rows, _ := db.Query(ctx, statement, sdk.TenantOrDefault(ctx), ids)
	held, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (loaded[T], error) {
		var one loaded[T]
		err := row.Scan(append([]any{&one.mail}, fields(&one.value)...)...)
		return one, err
	})
	if err != nil {
		return nil, fmt.Errorf("email: load %s: %w", what, err)
	}
	return held, nil
}

// failEach returns err for each of ids.
func failEach(ids []uuid.UUID, err error) []error {
	errs := make([]error, len(ids))
	for i := range errs {
		errs[i] = err
	}
	return errs
}

// oneEach answers each of ids with the one row held for it, errMailNotHeld for an id held has none for.
func oneEach[T any](ids []uuid.UUID, held []loaded[T], err error) ([]T, []error) {
	values := make([]T, len(ids))
	if err != nil {
		return values, failEach(ids, err)
	}
	byMail := make(map[uuid.UUID]T, len(held))
	for _, row := range held {
		byMail[row.mail] = row.value
	}
	errs := make([]error, len(ids))
	for i, id := range ids {
		value, found := byMail[id]
		if !found {
			errs[i] = errMailNotHeld
		}
		values[i] = value
	}
	return values, errs
}

// listEach answers each of ids with every row held for it in the order read, none for an id held has none for.
func listEach[T any](ids []uuid.UUID, held []loaded[T], err error) ([][]T, []error) {
	lists := make([][]T, len(ids))
	if err != nil {
		return lists, failEach(ids, err)
	}
	byMail := make(map[uuid.UUID][]T, len(ids))
	for _, row := range held {
		byMail[row.mail] = append(byMail[row.mail], row.value)
	}
	for i, id := range ids {
		lists[i] = byMail[id]
	}
	return lists, make([]error, len(ids))
}

// fetchParticipants loads the people each of the mails ids names, in one statement.
func (m mailLoaders) fetchParticipants(ctx context.Context, ids []uuid.UUID) ([][]participant, []error) {
	held, err := readLoaded(ctx, m.db, "the people of mail", `SELECT id, participants FROM plugin_email.messages
		WHERE tenant_id = $1 AND id = ANY($2)`,
		ids, whole[[]participant])
	return oneEach(ids, held, err)
}

// fetchBodies loads the text and safe HTML of each of the mails ids, in one statement.
func (m mailLoaders) fetchBodies(ctx context.Context, ids []uuid.UUID) ([]mailBody, []error) {
	held, err := readLoaded(ctx, m.db, "the bodies of mail", `SELECT id, body_text, safe_html FROM plugin_email.messages
		WHERE tenant_id = $1 AND id = ANY($2)`,
		ids, (*mailBody).fields)
	return oneEach(ids, held, err)
}

// fetchAttachments loads the files of each of the mails ids in position order, in one statement.
func (m mailLoaders) fetchAttachments(ctx context.Context, ids []uuid.UUID) ([][]attachmentView, []error) {
	held, err := readLoaded(ctx, m.db, "the files of mail", `SELECT message_id, id, position, filename,
			content_type, size, inline, content_id, state
		FROM plugin_email.attachments
		WHERE tenant_id = $1 AND message_id = ANY($2)
		ORDER BY position, id`,
		ids, (*attachmentView).fields)
	return listEach(ids, held, err)
}

// fetchContactIDs loads the contacts each of the mails ids is linked to in the order of their ids, in one statement.
func (m mailLoaders) fetchContactIDs(ctx context.Context, ids []uuid.UUID) ([][]uuid.UUID, []error) {
	held, err := readLoaded(ctx, m.db, "the contacts of mail", `SELECT message_id, contact_id
		FROM plugin_email.message_contacts
		WHERE tenant_id = $1 AND message_id = ANY($2)
		ORDER BY contact_id`,
		ids, whole[uuid.UUID])
	return listEach(ids, held, err)
}
