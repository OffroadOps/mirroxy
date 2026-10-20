package main

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// renameNode is called with a.mu held. Names do not affect running core configs.
func (a *App) renameNode(id, name string) error {
	n := a.node(id)
	if n == nil {
		return errors.New("节点不存在")
	}
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 128 {
		return errors.New("节点名称需为 1–128 个字符")
	}
	if strings.ContainsFunc(name, unicode.IsControl) {
		return errors.New("节点名称不能包含换行或控制字符")
	}
	if name == n.Name {
		return nil
	}
	before := *n
	if n.OriginalName == "" {
		n.OriginalName = n.Name
	}
	n.Name = name
	if n.Name == n.OriginalName {
		n.OriginalName = ""
	}
	if err := a.save(); err != nil {
		*n = before
		return err
	}
	return nil
}
