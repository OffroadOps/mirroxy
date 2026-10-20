package main

import "errors"

// Called with a.mu held. List placement is presentation metadata and must not
// reconcile proxy cores or change their ports, connections, or counters.
func (a *App) setNodeListBottom(id string, bottom bool) error {
	node := a.node(id)
	if node == nil {
		return errors.New("节点不存在")
	}
	if node.ListBottom == bottom {
		return nil
	}
	before := node.ListBottom
	node.ListBottom = bottom
	if err := a.save(); err != nil {
		node.ListBottom = before
		return err
	}
	return nil
}
