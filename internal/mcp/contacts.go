// SPDX-License-Identifier: Elastic-2.0

package mcp

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// errNoContact reports an id the directory holds no contact under.
var errNoContact = errors.New("mcp: AlphOne holds no contact under that id")

// contactPageDefault is how many contacts a search reads when none is asked for.
const contactPageDefault = 20

// contactPageMax bounds a search so the nested task probe stays under the cap.
const contactPageMax = 50

// contactTaskPage is how many open tasks one contact reads.
const contactTaskPage = 50

// contactsDocument searches the directory, probing each match for open work.
const contactsDocument = `query($q: String, $first: Int!) {
	contacts(q: $q, first: $first) {
		edges { node {
			id name
			identities { channel identifier displayName }
			tasks(status: "open", first: 1) { edges { node { id } } }
		} }
	}
}`

// contactDocument reads one contact with its addresses and open tasks.
const contactDocument = `query($id: UUID!, $first: Int!) {
	contact(id: $id) {
		id name createdAt
		identities { channel identifier displayName }
		tasks(status: "open", first: $first) {
			edges { node { id title dueOn status priority assigneeId } }
			pageInfo { hasNextPage }
		}
	}
}`

// identityNode is one channel address as the graph answers it.
type identityNode struct {
	Channel     string `json:"channel"`
	Identifier  string `json:"identifier"`
	DisplayName string `json:"displayName"`
}

// contactNode is one contact as the graph answers it.
type contactNode struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	CreatedAt  string         `json:"createdAt"`
	Identities []identityNode `json:"identities"`
	Tasks      struct {
		Edges []struct {
			Node taskNode `json:"node"`
		} `json:"edges"`
		PageInfo pageInfo `json:"pageInfo"`
	} `json:"tasks"`
}

// contactsData is what the search document answers with.
type contactsData struct {
	Contacts struct {
		Edges []struct {
			Node contactNode `json:"node"`
		} `json:"edges"`
	} `json:"contacts"`
}

// contactData is what the single contact document answers with.
type contactData struct {
	Contact *contactNode `json:"contact"`
}

// contactVariables resolves the arguments one search sends.
func contactVariables(in ContactsInput) map[string]any {
	variables := map[string]any{"first": boundedPage(in.Limit, contactPageDefault, contactPageMax)}
	if in.Query != "" {
		variables["q"] = in.Query
	}
	return variables
}

// boundedPage resolves a requested page size against its default and ceiling.
func boundedPage(requested, fallback, ceiling int) int {
	if requested <= 0 {
		return fallback
	}
	return min(requested, ceiling)
}

// toIdentities maps answered addresses onto the list an agent reads.
func toIdentities(nodes []identityNode) []Identity {
	held := make([]Identity, 0, len(nodes))
	for _, node := range nodes {
		held = append(held, Identity(node))
	}
	return held
}

// contacts searches the directory, marking who holds open work.
func (t *tools) contacts(ctx context.Context, in ContactsInput) (*mcp.CallToolResult, ContactsOutput, error) {
	var data contactsData
	if err := t.execute(ctx, contactsDocument, contactVariables(in), &data); err != nil {
		return nil, ContactsOutput{}, err
	}
	out := ContactsOutput{Contacts: make([]ContactItem, 0, len(data.Contacts.Edges))}
	for _, edge := range data.Contacts.Edges {
		out.Contacts = append(out.Contacts, ContactItem{
			ID:           edge.Node.ID,
			Name:         edge.Node.Name,
			Identities:   toIdentities(edge.Node.Identities),
			HasOpenTasks: len(edge.Node.Tasks.Edges) > 0,
		})
	}
	return nil, out, nil
}

// contact reports one contact in full.
func (t *tools) contact(ctx context.Context, in ContactInput) (*mcp.CallToolResult, ContactOutput, error) {
	var data contactData
	variables := map[string]any{"id": in.ContactID, "first": contactTaskPage}
	if err := t.execute(ctx, contactDocument, variables, &data); err != nil {
		return nil, ContactOutput{}, err
	}
	if data.Contact == nil {
		return nil, ContactOutput{}, errNoContact
	}
	out := ContactOutput{
		ID:              data.Contact.ID,
		Name:            data.Contact.Name,
		CreatedAt:       data.Contact.CreatedAt,
		Identities:      toIdentities(data.Contact.Identities),
		OpenTasks:       make([]TaskItem, 0, len(data.Contact.Tasks.Edges)),
		OpenTasksCapped: data.Contact.Tasks.PageInfo.HasNextPage,
	}
	for _, edge := range data.Contact.Tasks.Edges {
		out.OpenTasks = append(out.OpenTasks, toTaskItem(edge.Node))
	}
	return nil, out, nil
}
