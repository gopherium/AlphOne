// SPDX-License-Identifier: Elastic-2.0

package dyngraph

// BuildLocks reports how many tenants hold a build lock.
func (g *Graphs[T]) BuildLocks() int {
	g.locking.Lock()
	defer g.locking.Unlock()
	return len(g.locks)
}
