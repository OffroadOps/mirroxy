package main

import (
	"errors"
	"net/url"
	"strings"
)

// validSubscriptionURL accepts only absolute HTTP(S) addresses, the same rule
// /api/subscription/add applies.
func validSubscriptionURL(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Host != "" && (u.Scheme == "https" || u.Scheme == "http")
}

// subscriptionHost is the only part of a subscription address shown in the
// panel state; the path and query usually carry the provider's token.
func subscriptionHost(raw string) string {
	u, e := url.Parse(raw)
	if e != nil {
		return ""
	}
	return u.Hostname()
}

// editSubscription renames a subscription and, when rawURL is not empty,
// replaces its address. Nodes, ports and custom names are untouched; the new
// address takes effect on the next refresh. Callers hold a.mu.
func (a *App) editSubscription(id, name, rawURL string) error {
	name, rawURL = strings.TrimSpace(name), strings.TrimSpace(rawURL)
	if name == "" {
		return errors.New("请填写订阅名称")
	}
	if rawURL != "" && !validSubscriptionURL(rawURL) {
		return errors.New("请填写有效的 HTTP / HTTPS 订阅地址")
	}
	for i := range a.cfg.Subscriptions {
		s := &a.cfg.Subscriptions[i]
		if s.ID != id {
			continue
		}
		before := *s
		s.Name = name
		if rawURL != "" && rawURL != s.URL {
			s.URL, s.Error = rawURL, ""
		}
		if e := a.save(); e != nil {
			*s = before
			return e
		}
		a.event("订阅已修改：" + name)
		return nil
	}
	return errors.New("订阅不存在")
}

// deleteSubscription removes a subscription. With keepNodes its nodes stay as
// manually added nodes (ports, names and running state unchanged); otherwise
// they are deleted and their cores stop. Other nodes are never restarted.
// Callers hold a.mu.
func (a *App) deleteSubscription(id string, keepNodes bool) error {
	index := -1
	for i, s := range a.cfg.Subscriptions {
		if s.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return errors.New("订阅不存在")
	}
	if !keepNodes && a.busyUpdate {
		return errors.New("核心更新中，请稍后")
	}
	if !keepNodes {
		for _, n := range a.cfg.Nodes {
			if n.Source == id && a.appRouterUsesNode(n.ID) {
				return errors.New("此订阅的节点正被应用代理使用，请先停止应用代理")
			}
		}
	}
	before := cloneSettings(a.cfg)
	name := a.cfg.Subscriptions[index].Name
	a.cfg.Subscriptions = append(a.cfg.Subscriptions[:index:index], a.cfg.Subscriptions[index+1:]...)
	nodes := a.cfg.Nodes[:0:0]
	for _, n := range a.cfg.Nodes {
		if n.Source == id {
			if !keepNodes {
				continue
			}
			n.Source = ""
		}
		nodes = append(nodes, n)
	}
	a.cfg.Nodes = nodes
	if !keepNodes {
		if e := a.reconcile(); e != nil {
			a.cfg = before
			_ = a.reconcile()
			return e
		}
	}
	if e := a.save(); e != nil {
		a.cfg = before
		if !keepNodes {
			_ = a.reconcile()
		}
		return e
	}
	if !keepNodes {
		a.syncProxy()
	}
	a.event("订阅已删除：" + name)
	return nil
}
