// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

// fieldName is a contact detail a column may be mapped onto.
type fieldName string

// The fields an import may fill.
const (
	fieldContactName fieldName = "name"
	fieldEmail       fieldName = "email"
	fieldPhone       fieldName = "phone"
)

// mappableField is one field of the registry, as the graph reports it.
type mappableField struct {
	Name     fieldName
	Label    string
	Required bool
}

// coreFields lists the columns the importer serves without any provider.
var coreFields = []mappableField{
	{Name: fieldContactName, Label: "Name", Required: true},
	{Name: fieldEmail, Label: "Email", Required: false},
	{Name: fieldPhone, Label: "Phone", Required: false},
}

// core reports whether the name is one of the importer's own columns.
func core(name fieldName) bool {
	return slices.ContainsFunc(coreFields, func(field mappableField) bool {
		return field.Name == name
	})
}

// registry is the mappable registry beside the provider serving each field.
type registry struct {
	fields    []mappableField
	owner     map[fieldName]int
	providers []sdk.FieldProvider
}

// UseFieldProviders receives the providers serving runtime defined fields.
func (p *Plugin) UseFieldProviders(providers []sdk.FieldProvider) {
	p.providers = providers
}

// registry merges the core columns with every field the providers serve.
func (p *Plugin) registry(ctx context.Context) (registry, error) {
	held := registry{
		fields:    slices.Clone(coreFields),
		owner:     map[fieldName]int{},
		providers: p.providers,
	}
	for index, provider := range p.providers {
		listed, err := provider.LiveContactFields(ctx)
		if err != nil {
			return registry{}, err
		}
		for _, field := range listed {
			held.add(fieldName(field.Name), field.Label, index)
		}
	}
	return held, nil
}

// add lists one provider served field unless its name is already claimed.
func (r *registry) add(name fieldName, label string, index int) {
	if core(name) {
		return
	}
	if _, claimed := r.owner[name]; claimed {
		return
	}
	r.owner[name] = index
	r.fields = append(r.fields, mappableField{Name: name, Label: label})
}

// holds reports whether the registry carries the named field.
func (r registry) holds(name fieldName) bool {
	if core(name) {
		return true
	}
	_, served := r.owner[name]
	return served
}

// checkTexts reports whether every provider accepts the texts its own fields carry.
func (r registry) checkTexts(ctx context.Context, texts map[string]string) error {
	grouped, err := r.group(texts)
	if err != nil {
		return err
	}
	for index, held := range grouped {
		if err := r.providers[index].CheckContactFieldTexts(ctx, held); err != nil {
			return err
		}
	}
	return nil
}

// writeTexts stores each text through the provider serving its field.
func (r registry) writeTexts(ctx context.Context, contactID uuid.UUID, texts map[string]string) error {
	grouped, err := r.group(texts)
	if err != nil {
		return err
	}
	for index, held := range grouped {
		if err := r.providers[index].WriteContactFieldTexts(ctx, contactID, held); err != nil {
			return err
		}
	}
	return nil
}

// group sorts the texts by the provider serving each field, refusing the unserved.
func (r registry) group(texts map[string]string) (map[int]map[string]string, error) {
	grouped := make(map[int]map[string]string, len(texts))
	var unserved []string
	for name, text := range texts {
		index, served := r.owner[fieldName(name)]
		if !served {
			unserved = append(unserved, name)
			continue
		}
		if grouped[index] == nil {
			grouped[index] = map[string]string{}
		}
		grouped[index][name] = text
	}
	if len(unserved) > 0 {
		slices.Sort(unserved)
		return nil, fmt.Errorf("%w: no live field holds %s",
			sdk.ErrInvalidFieldText, strings.Join(unserved, ", "))
	}
	return grouped, nil
}
