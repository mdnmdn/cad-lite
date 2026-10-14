package commands

import (
	"runtime"
	"sync"
	"weak"

	"cad-lite/cmdsys"
	"cad-lite/doc"
)

// erasedMemory is what OOPS restores: the entities the latest ERASE removed, per drawing. A drawing
// that is no longer reachable drops its entry.
var erasedMemory = struct {
	sync.Mutex
	m map[weak.Pointer[doc.Drawing]][]doc.Entity
}{m: map[weak.Pointer[doc.Drawing]][]doc.Entity{}}

// rememberErased records copies of the entities hs, about to be erased, as the set OOPS restores.
// An empty set leaves the memory unchanged. A command that erases objects on the user's behalf
// (ERASE) calls it before the erase.
func rememberErased(ctx *cmdsys.Context, hs []doc.Handle) {
	var ents []doc.Entity
	for _, h := range hs {
		if e := ctx.Doc.Entity(h); e != nil {
			ents = append(ents, copyOf(e))
		}
	}
	if len(ents) == 0 {
		return
	}
	key := weak.Make(ctx.Doc)
	erasedMemory.Lock()
	defer erasedMemory.Unlock()
	if _, ok := erasedMemory.m[key]; !ok {
		runtime.AddCleanup(ctx.Doc, func(k weak.Pointer[doc.Drawing]) {
			erasedMemory.Lock()
			delete(erasedMemory.m, k)
			erasedMemory.Unlock()
		}, key)
	}
	erasedMemory.m[key] = ents
}

// takeErased returns the set OOPS restores and forgets it.
func takeErased(d *doc.Drawing) []doc.Entity {
	key := weak.Make(d)
	erasedMemory.Lock()
	defer erasedMemory.Unlock()
	ents := erasedMemory.m[key]
	erasedMemory.m[key] = nil
	return ents
}

// registerOops adds OOPS: it restores the objects the last ERASE removed, as new entities (the
// handles are new, the layer and properties are kept). It is one undo step.
func registerOops(r *cmdsys.Registry) {
	r.Register("OOPS", func() cmdsys.Command {
		return &immediate{name: "OOPS", run: func(ctx *cmdsys.Context) {
			ents := takeErased(ctx.Doc)
			if len(ents) == 0 {
				ctx.Say("cmd.oops.msg.nothing")
				return
			}
			for _, e := range ents {
				ctx.Tx().Add(e)
			}
		}}
	})
}
