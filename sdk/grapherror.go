// SPDX-License-Identifier: Elastic-2.0

package sdk

// GraphError classifies a plugin resolver error for the graph presenter.
type GraphError struct {
	// Code is the extensions code the presenter reports.
	Code string
	// Reason is the stable snake_case name a client translates the error by.
	Reason string
	// Meta carries the named values the reason's message interpolates.
	Meta map[string]any
	// Extensions carries extra extension fields beside the code.
	Extensions map[string]any
	// Err is the underlying error whose message the client reads.
	Err error
}

// Error returns the underlying error message.
func (e GraphError) Error() string {
	return e.Err.Error()
}

// Unwrap returns the underlying error.
func (e GraphError) Unwrap() error {
	return e.Err
}
