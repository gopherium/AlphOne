// SPDX-License-Identifier: Elastic-2.0

package whatsapp_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// retiredConversation stands in for a conversation id in a retired path.
const retiredConversation = "019f4a00-0000-7000-8000-000000000001"

func TestRoutesServeOnlyTheWebhookPairAndTheMediaDownload(t *testing.T) {
	t.Parallel()

	routes := newPlugin(t, unreachableDatabaseURL, nil, nil).Routes()

	for name, target := range map[string]struct {
		method string
		path   string
	}{
		"the conversation list": {http.MethodGet, "/conversations"},
		"the message list":      {http.MethodGet, "/conversations/" + retiredConversation + "/messages"},
		"the message send":      {http.MethodPost, "/conversations/" + retiredConversation + "/messages"},
	} {
		recorder := httptest.NewRecorder()
		routes.ServeHTTP(recorder, httptest.NewRequest(target.method, target.path, nil))

		if recorder.Code != http.StatusNotFound {
			t.Errorf("%s status = %d, want %d", name, recorder.Code, http.StatusNotFound)
		}
	}
}
