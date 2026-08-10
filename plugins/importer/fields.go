// SPDX-License-Identifier: Elastic-2.0

package importer

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

// mappableFields lists every field a column may be mapped onto.
var mappableFields = []mappableField{
	{Name: fieldContactName, Label: "Name", Required: true},
	{Name: fieldEmail, Label: "Email", Required: false},
	{Name: fieldPhone, Label: "Phone", Required: false},
}

// mappable reports whether the registry carries the named field.
func mappable(name fieldName) bool {
	for _, field := range mappableFields {
		if field.Name == name {
			return true
		}
	}
	return false
}
