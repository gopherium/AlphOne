// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"context"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/sdk"
)

// routesRoot is the path the host serves the routes of the plugin under.
const routesRoot = "/api/plugins/" + pluginID

// mapEach maps each of values onto the graph through toGraph.
func mapEach[V, G any](values []V, toGraph func(V) G) []G {
	mapped := make([]G, len(values))
	for i, value := range values {
		mapped[i] = toGraph(value)
	}
	return mapped
}

// answerRead returns the mapping of what a read answered onto the graph through toGraph, or the error of the read.
func answerRead[V, G any](toGraph func(V) G) func(V, error) (G, error) {
	return func(read V, err error) (G, error) {
		if err != nil {
			var none G
			return none, err
		}
		return toGraph(read), nil
	}
}

// answerPage reads the page limit and offset name through read and maps it onto the graph through toGraph of its size.
func answerPage[V, G any](
	bounds sdk.PageBounds, limit, offset *int, toGraph func(size int) func(V) G, read func(size, start int) (V, error),
) (G, error) {
	size, err := bounds.Limit(limit)
	if err != nil {
		var none G
		return none, err
	}
	return answerRead(toGraph(size))(read(size, sdk.Offset(offset)))
}

// answerHeld returns the mapping of what a read of one row answered onto the graph, nil when the row is not held.
func answerHeld[V, G any](toGraph func(V) *G) func(V, bool, error) (*G, error) {
	return func(read V, held bool, err error) (*G, error) {
		if err != nil || !held {
			return nil, err
		}
		return toGraph(read), nil
	}
}

// keptPath returns path for a file whose state says it is kept, nil for any other state.
func keptPath(state, path string) *string {
	if state != fileStored {
		return nil
	}
	return &path
}

// toGraphMailbox maps a mailbox onto its graph model.
func toGraphMailbox(box mailbox) *model.EmailMailbox {
	return &model.EmailMailbox{ID: box.id, Address: box.address, Aliases: box.aliases}
}

// toGraphThread maps a thread beside its newest mail onto its graph model.
func toGraphThread(thread threadView) *model.EmailThread {
	return &model.EmailThread{
		ID: thread.id, Subject: thread.subject, FirstMessageAt: thread.firstMessageAt,
		LastMessageAt: thread.lastMessageAt, MessageCount: thread.messageCount, HasAttachments: thread.hasAttachments,
		Latest: toGraphMail(thread.latest),
	}
}

// toGraphThreadPage returns the mapping of a page of the inbox onto its graph model, answering limit as its size.
func toGraphThreadPage(limit int) func(threadPage) *model.EmailThreadPage {
	return func(page threadPage) *model.EmailThreadPage {
		return &model.EmailThreadPage{Items: mapEach(page.items, toGraphThread), Total: page.total, Limit: limit}
	}
}

// toGraphMail maps a mail of a light read onto its graph model.
func toGraphMail(mail mailView) *model.EmailMessage {
	return &model.EmailMessage{
		ID: mail.id, ThreadID: mail.threadID, Direction: mail.direction, Subject: mail.subject,
		From: toGraphAddress(mail.from), SentAt: mail.sentAt, ReceivedAt: mail.receivedAt, Snippet: mail.snippet,
		BodyState: mail.bodyState, SkippedParts: mail.skippedParts, Automation: mail.automation,
		Encrypted: mail.encrypted, Signed: mail.signed, Winmail: mail.winmail, InviteMethod: mail.inviteMethod,
		RawState: mail.rawState, RawSize: float64(mail.rawSize),
		OriginalPath: keptPath(mail.rawState, routesRoot+"/messages/"+mail.id.String()+"/original"),
	}
}

// toGraphMails maps the mails of a light read onto their graph models.
func toGraphMails(mails []mailView) []*model.EmailMessage {
	return mapEach(mails, toGraphMail)
}

// toGraphMailList returns the mapping of a page of the mail of a thread onto its graph models.
func toGraphMailList(int) func([]mailView) []*model.EmailMessage {
	return toGraphMails
}

// toGraphMailPage returns the mapping of a page of mail onto its graph model, answering limit as its size.
func toGraphMailPage(limit int) func(mailPage) *model.EmailMessagePage {
	return func(page mailPage) *model.EmailMessagePage {
		return &model.EmailMessagePage{Items: toGraphMails(page.items), Total: page.total, Limit: limit}
	}
}

// toGraphAddress maps the first sender of a mail onto its graph model, nil when the mail names none.
func toGraphAddress(from *participant) *model.EmailAddress {
	if from == nil {
		return nil
	}
	return &model.EmailAddress{Name: from.Name, Email: from.Email}
}

// toGraphParticipants maps the people a mail names onto their graph models.
func toGraphParticipants(people []participant) []*model.EmailParticipant {
	return mapEach(people, func(person participant) *model.EmailParticipant {
		return &model.EmailParticipant{Role: person.Role, Name: person.Name, Email: person.Email}
	})
}

// toGraphBody maps the text and the safe HTML of a mail onto their graph model.
func toGraphBody(body mailBody) *model.EmailBody {
	return &model.EmailBody{Text: body.text, HTML: body.html}
}

// toGraphAttachments maps the files of a mail onto their graph models.
func toGraphAttachments(files []attachmentView) []*model.EmailAttachment {
	return mapEach(files, func(file attachmentView) *model.EmailAttachment {
		return &model.EmailAttachment{
			ID: file.id, Position: file.position, Filename: file.filename, ContentType: file.contentType,
			Size: float64(file.size), Inline: file.inline, ContentID: file.contentID, State: file.state,
			DownloadPath: keptPath(file.state, routesRoot+"/attachments/"+file.id.String()),
		}
	})
}

// QueryResolvers serves the plugin's Query root fields.
type QueryResolvers struct {
	plugin *Plugin
}

// QueryResolvers returns the plugin's Query resolver set.
func (p *Plugin) QueryResolvers() QueryResolvers {
	return QueryResolvers{plugin: p}
}

// EmailMailbox reads the mailbox of the calling workspace, nil when it holds none.
func (q QueryResolvers) EmailMailbox(ctx context.Context) (*model.EmailMailbox, error) {
	return answerHeld(toGraphMailbox)(readWorkspaceMailbox(ctx, q.plugin.pool, sdk.TenantOrDefault(ctx)))
}

// EmailThreads lists one page of the threads of the calling workspace by their newest mail, beside how many it holds.
func (q QueryResolvers) EmailThreads(ctx context.Context, limit, offset *int) (*model.EmailThreadPage, error) {
	return answerPage(q.plugin.pages, limit, offset, toGraphThreadPage, func(size, start int) (threadPage, error) {
		return readInbox(ctx, q.plugin.pool, sdk.TenantOrDefault(ctx), size, start)
	})
}

// EmailThread reads one thread of the calling workspace, nil for an unknown, foreign or merged id.
func (q QueryResolvers) EmailThread(ctx context.Context, id uuid.UUID) (*model.EmailThread, error) {
	return answerHeld(toGraphThread)(readThread(ctx, q.plugin.pool, sdk.TenantOrDefault(ctx), id))
}

// EmailContactMessages lists one page of the mail linked to a contact of the calling workspace, newest first.
func (q QueryResolvers) EmailContactMessages(
	ctx context.Context, contactID uuid.UUID, limit, offset *int,
) (*model.EmailMessagePage, error) {
	return answerPage(q.plugin.pages, limit, offset, toGraphMailPage, func(size, start int) (mailPage, error) {
		return readContactMails(ctx, q.plugin.pool, sdk.TenantOrDefault(ctx), contactID, size, start)
	})
}

// EmailThreadResolvers serves the thread's resolver backed fields.
type EmailThreadResolvers struct {
	plugin *Plugin
}

// EmailThreadResolvers returns the plugin's thread resolver set.
func (p *Plugin) EmailThreadResolvers() EmailThreadResolvers {
	return EmailThreadResolvers{plugin: p}
}

// Messages lists one page of the mail of the thread, newest first.
func (r EmailThreadResolvers) Messages(
	ctx context.Context, obj *model.EmailThread, limit, offset *int,
) ([]*model.EmailMessage, error) {
	return answerPage(r.plugin.pages, limit, offset, toGraphMailList, func(size, start int) ([]mailView, error) {
		return readThreadMails(ctx, r.plugin.pool, sdk.TenantOrDefault(ctx), obj.ID, size, start)
	})
}

// Message reads one mail of the thread, nil for a mail of another thread or workspace.
func (r EmailThreadResolvers) Message(
	ctx context.Context, obj *model.EmailThread, id uuid.UUID,
) (*model.EmailMessage, error) {
	return answerHeld(toGraphMail)(readThreadMail(ctx, r.plugin.pool, sdk.TenantOrDefault(ctx), obj.ID, id))
}

// EmailMessageResolvers serves the mail's resolver backed fields.
type EmailMessageResolvers struct {
	plugin *Plugin
}

// EmailMessageResolvers returns the plugin's mail resolver set.
func (p *Plugin) EmailMessageResolvers() EmailMessageResolvers {
	return EmailMessageResolvers{plugin: p}
}

// Participants lists every address the mail names beside its field, within the bound of lists one request reads.
func (r EmailMessageResolvers) Participants(
	ctx context.Context, obj *model.EmailMessage,
) ([]*model.EmailParticipant, error) {
	return answerRead(toGraphParticipants)(r.plugin.loaders.participants(ctx, obj.ID))
}

// Body reads the text and the safe HTML of the mail, within the bound of bodies one request reads.
func (r EmailMessageResolvers) Body(ctx context.Context, obj *model.EmailMessage) (*model.EmailBody, error) {
	return answerRead(toGraphBody)(r.plugin.loaders.body(ctx, obj.ID))
}

// Attachments lists the files of the mail in position order, within the bound of lists one request reads.
func (r EmailMessageResolvers) Attachments(
	ctx context.Context, obj *model.EmailMessage,
) ([]*model.EmailAttachment, error) {
	return answerRead(toGraphAttachments)(r.plugin.loaders.attachments(ctx, obj.ID))
}

// ContactIds names the contacts the mail is linked to.
func (r EmailMessageResolvers) ContactIds(ctx context.Context, obj *model.EmailMessage) ([]uuid.UUID, error) {
	return r.plugin.loaders.contactIDs(ctx, obj.ID)
}
