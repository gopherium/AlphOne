// SPDX-License-Identifier: Elastic-2.0

package dyngraph

// BuildLocks reports how many tenants hold a build lock.
func (g *Graphs[T]) BuildLocks() int {
	locks := 0
	g.building.Range(func(any, any) bool {
		locks++
		return true
	})
	return locks
}
