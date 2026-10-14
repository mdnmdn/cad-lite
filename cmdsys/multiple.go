package cmdsys

// CommitStep commits the command's transaction now and leaves the command running, so the work
// done so far is one undo step and what follows is the next. MULTIPLE calls it between two runs
// of the command it repeats. It does nothing when the command has not touched the document.
func (c *Context) CommitStep() {
	if c.txn == nil {
		return
	}
	if cs := c.txn.Commit(); len(cs.Added) > 0 {
		c.SetLastEntity(cs.Added[len(cs.Added)-1])
	}
	c.txn = nil
}
