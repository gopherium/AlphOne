// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gopherium/alphone/sdk"
)

// errGateDown is the failure of a tenant gate that cannot read its workspace.
var errGateDown = errors.New("tenant gate unreachable")

// errMatchDown is the failure of a contact match that cannot reach the contacts.
var errMatchDown = errors.New("contact match unreachable")

// errSettingsDown is the failure of a settings store that cannot be reached.
var errSettingsDown = errors.New("settings store unreachable")

// hostCall is one call a fake of an ingest test saw, beside the workspace its context named.
type hostCall struct {
	// step names the call.
	step string
	// tenant is the workspace the call's context named, uuid.Nil when it named none.
	tenant uuid.UUID
}

// callLog lists in order the calls the fakes of one ingest test saw.
type callLog struct {
	// mu guards calls.
	mu sync.Mutex
	// calls lists every call in order.
	calls []hostCall
}

// add records step under the workspace ctx names.
func (l *callLog) add(ctx context.Context, step string) {
	tenant, _ := sdk.TenantFromContext(ctx)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, hostCall{step: step, tenant: tenant})
}

// all returns every call so far.
func (l *callLog) all() []hostCall {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.calls)
}

// steps returns the step of every call so far.
func (l *callLog) steps() []string {
	var steps []string
	for _, call := range l.all() {
		steps = append(steps, call.step)
	}
	return steps
}

// fakeGate answers the same for every workspace whether it takes mail, running hook ahead of each answer.
type fakeGate struct {
	// log records each call.
	log *callLog
	// closed reports a workspace that takes no mail.
	closed bool
	// err replaces the answer when set.
	err error
	// hook runs ahead of each answer when set.
	hook func()
}

// AcceptsMachineTraffic records the call, runs hook and answers whether the workspace is open.
func (g *fakeGate) AcceptsMachineTraffic(ctx context.Context, _ uuid.UUID) (bool, error) {
	g.log.add(ctx, "gate")
	if g.hook != nil {
		g.hook()
	}
	return !g.closed, g.err
}

// fakeMatcher answers the contacts it knows in the workspace a call's context names, recording every batch.
type fakeMatcher struct {
	// log records each call.
	log *callLog
	// known maps each workspace to the contact each email it knows names.
	known map[uuid.UUID]map[string]uuid.UUID
	// err replaces the answer when set.
	err error
	// hook runs ahead of each answer when set.
	hook func()
	// mu guards batches and channels.
	mu sync.Mutex
	// batches lists the identifiers of each call in order.
	batches [][]string
	// channels lists the channel of each call in order.
	channels []sdk.Channel
}

// MatchIdentities records the batch, runs hook and answers each identifier the workspace of ctx knows.
func (m *fakeMatcher) MatchIdentities(
	ctx context.Context, channel sdk.Channel, identifiers []string,
) (map[string]sdk.Contact, error) {
	m.log.add(ctx, "match")
	m.mu.Lock()
	m.batches = append(m.batches, slices.Clone(identifiers))
	m.channels = append(m.channels, channel)
	m.mu.Unlock()
	if m.hook != nil {
		m.hook()
	}
	if m.err != nil {
		return nil, m.err
	}
	matched := map[string]sdk.Contact{}
	for _, identifier := range identifiers {
		if contact, found := m.known[sdk.TenantOrDefault(ctx)][identifier]; found {
			matched[identifier] = sdk.Contact{ID: contact}
		}
	}
	return matched, nil
}

// calls returns the batches every call so far took.
func (m *fakeMatcher) calls() [][]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.batches)
}

// savingStore hands every call to the host's file store, recording each SaveAs and its type and running hook first.
type savingStore struct {
	sdk.FileStore
	// log records each SaveAs.
	log *callLog
	// hook runs ahead of each SaveAs when set, and its error replaces the save.
	hook func(ctx context.Context, id uuid.UUID) error
	// mu guards types.
	mu sync.Mutex
	// types lists the media type of each SaveAs in order.
	types []string
}

// SaveAs records the call, runs hook and saves body through the host's store unless hook fails.
func (s *savingStore) SaveAs(ctx context.Context, id uuid.UUID, contentType string, body io.Reader) (
	sdk.StoredFile, error) {
	s.log.add(ctx, "save")
	s.mu.Lock()
	s.types = append(s.types, contentType)
	s.mu.Unlock()
	if s.hook != nil {
		if err := s.hook(ctx, id); err != nil {
			return sdk.StoredFile{}, err
		}
	}
	return s.FileStore.SaveAs(ctx, id, contentType, body)
}

// savedTypes returns the media type of every SaveAs so far.
func (s *savingStore) savedTypes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.types)
}

// steppingPublisher records every event it is handed, running hook first.
type steppingPublisher struct {
	// log records each event.
	log *callLog
	// hook runs ahead of recording each event when set.
	hook func(ctx context.Context, data map[string]any)
	// mu guards events.
	mu sync.Mutex
	// events lists the events in the order they came.
	events []published
}

// Publish records the call, runs hook and records the event beside the workspace its context names.
func (p *steppingPublisher) Publish(ctx context.Context, name string, data map[string]any) {
	p.log.add(ctx, "publish")
	if p.hook != nil {
		p.hook(ctx, data)
	}
	tenant, _ := sdk.TenantFromContext(ctx)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, published{tenant: tenant, name: name, data: data})
}

// failingSettings is a settings store whose every lookup fails.
type failingSettings struct {
	sdk.SettingStore
}

// Lookup answers errSettingsDown.
func (failingSettings) Lookup(context.Context, string) (string, bool, error) {
	return "", false, errSettingsDown
}

// batchFailingTx is a failingTx that also sends injectedFailure in place of a batch holding its fragment.
type batchFailingTx struct {
	failingTx
}

// SendBatch sends batch, or a batch of injectedFailure alone when a statement of batch holds the fragment.
func (f batchFailingTx) SendBatch(ctx context.Context, batch *pgx.Batch) pgx.BatchResults {
	for _, queued := range batch.QueuedQueries {
		if strings.Contains(queued.SQL, f.fragment) {
			failing := &pgx.Batch{}
			failing.Queue(injectedFailure)
			return f.Tx.SendBatch(ctx, failing)
		}
	}
	return f.Tx.SendBatch(ctx, batch)
}
