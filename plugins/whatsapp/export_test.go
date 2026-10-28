// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import "github.com/gopherium/alphone/sdk"

// Subscribers returns how many live listeners the broadcaster holds.
func (p *Plugin) Subscribers() int {
	p.events.mu.Lock()
	defer p.events.mu.Unlock()
	return len(p.events.subs)
}

// BoundEncrypter returns an encrypter that ties each value to the calling tenant and purpose without a key.
func BoundEncrypter() sdk.Encrypter {
	return &boundEncrypter{}
}
