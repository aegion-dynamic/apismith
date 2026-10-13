package request

import "sync"

// Vars is the process-local variable table used by {{name}} and sequence captures.
// Values stay in memory. A captured token is not written to history or the library.
type Vars struct {
	mu sync.RWMutex
	m  map[string]string
}

// NewVars returns an empty table.
func NewVars() *Vars {
	return &Vars{m: map[string]string{}}
}

// Snapshot returns a copy of the table.
func (v *Vars) Snapshot() map[string]string {
	if v == nil {
		return map[string]string{}
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	return cloneMap(v.m)
}

// Replace swaps the table for a copy of m. Nil becomes empty.
func (v *Vars) Replace(m map[string]string) {
	if v == nil {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.m = cloneMap(m)
}

// Get returns one value.
func (v *Vars) Get(name string) (string, bool) {
	if v == nil {
		return "", false
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	val, ok := v.m[name]
	return val, ok
}

// Delete removes one name.
func (v *Vars) Delete(name string) {
	if v == nil {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.m, name)
}

func cloneMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
