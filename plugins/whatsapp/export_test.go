// SPDX-License-Identifier: Elastic-2.0

package whatsapp

// Subscribers returns how many live listeners the broadcaster holds.
func (p *Plugin) Subscribers() int {
	p.events.mu.Lock()
	defer p.events.mu.Unlock()
	return len(p.events.subs)
}
