// SPDX-License-Identifier: Elastic-2.0

package importer

import "errors"

// errTooLarge reports an upload beyond the size cap.
var errTooLarge = errors.New("the file is larger than the importer accepts")
