// SPDX-License-Identifier: Elastic-2.0

package sdk

// The reasons a [FieldTextError] names that a field consumer reads.
const (
	// FieldTextKindMismatch names a text that does not fit the kind its one field declares.
	FieldTextKindMismatch = "value_kind_mismatch"
	// FieldTextFieldUnknown names texts for fields no live definition holds.
	FieldTextFieldUnknown = "field_unknown"
)

// FieldTextError reports field texts a provider will not store, naming the fault as data.
type FieldTextError struct {
	// Reason is the stable snake_case name of the fault.
	Reason string
	// Fields names the fields at fault, sorted.
	Fields []string
	// Kind is the kind the one named field declares, empty when the reason names none.
	Kind string
	// Err is the provider's own error.
	Err error
}

// Error returns the sentinel text beside the provider's own message.
func (e FieldTextError) Error() string {
	return ErrInvalidFieldText.Error() + ": " + e.Err.Error()
}

// Unwrap returns the sentinel beside the provider's own error.
func (e FieldTextError) Unwrap() []error {
	return []error{ErrInvalidFieldText, e.Err}
}
