package main

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// nodeEditableSpec returns the flat form data for the node editor, with the
// provider's original settings when a subscription refresh would otherwise
// revert an edit.
func (a *App) nodeEditableSpec(id string) (M, error) {
	n := a.node(id)
	if n == nil {
		return nil, errors.New("节点不存在")
	}
	s, err := nodeSpec(*n)
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(s)
	var form M
	_ = json.Unmarshal(b, &form)
	if n.Source != "" {
		if sub := a.subscription(n.Source); sub != nil {
			form["subscription"] = sub.Name
		} else {
			form["source"] = n.Source
		}
	}
	return form, nil
}

// nodeEditableSpecError returns the failure of the spec extraction separately
// so the API can answer with the form payload and error in one lock pass.
func (a *App) nodeEditableSpecError(id string) error {
	_, err := a.nodeEditableSpec(id)
	return err
}

func (a *App) subscription(id string) *Subscription {
	for i := range a.cfg.Subscriptions {
		if a.cfg.Subscriptions[i].ID == id {
			return &a.cfg.Subscriptions[i]
		}
	}
	return nil
}

// editNode applies editor form fields to a stored node. It is called with
// a.mu held. Local port, enabled state and the custom name are never touched.
func (a *App) editNode(id string, form M) error {
	if a.busyUpdate {
		return errors.New("核心更新中，请稍后")
	}
	n := a.node(id)
	if n == nil {
		return errors.New("节点不存在")
	}
	s, err := nodeSpec(*n)
	if err != nil {
		return err
	}
	trim := func(key string) { if v, ok := form[key].(string); ok { s.set(key, strings.TrimSpace(v)) } }
	trim("name")
	trim("server")
	if v, ok := form["port"].(string); ok {
		s.set("port", v)
	} else if v, ok := form["port"].(float64); ok {
		s.set("port", strconv.Itoa(int(v)))
	}
	for _, key := range []string{"password", "username", "method", "flow", "encryption", "network", "security", "sni", "fingerprint", "publicKey", "shortId", "path", "host", "serviceName", "alpn"} {
		trim(key)
	}
	if v, ok := form["alterId"].(float64); ok {
		s.AlterID = int(v)
	}
	if v, ok := form["insecure"].(bool); ok {
		s.Insecure = v
	}
	fresh, err := makeNode(s)
	if err != nil {
		return err
	}
	if fresh.Engine != n.Engine {
		return errors.New("协议与核心不匹配，无法保存")
	}
	before := *n
	updated := *n
	updated.Outbound = fresh.Outbound
	updated.Server = fresh.Server
	if form["name"] != nil {
		name := strings.TrimSpace(stringVal(form["name"]))
		if name == "" {
			return errors.New("节点名称不能为空")
		}
		if err := validateNodeName(name); err != nil {
			return err
		}
		if n.OriginalName == "" {
			updated.OriginalName = n.Name
		}
		updated.Name = name
		if updated.Name == updated.OriginalName {
			updated.OriginalName = ""
		}
	}
	if n.Source != "" {
		spec := cloneSpec(s)
		updated.EditSpec = &spec
	}
	*n = updated
	if err := a.reconcile(); err != nil {
		*n = before
		if e := a.reconcile(); e != nil {
			a.event("恢复旧配置失败：" + e.Error())
		}
		return err
	}
	if err := a.save(); err != nil {
		*n = before
		_ = a.reconcile()
		return err
	}
	a.syncProxy()
	a.event("节点已编辑：" + n.Name)
	return nil
}

func validateNodeName(name string) error {
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 128 {
		return errors.New("节点名称需为 1–128 个字符")
	}
	if strings.ContainsFunc(name, unicode.IsControl) {
		return errors.New("节点名称不能包含换行或控制字符")
	}
	return nil
}

// set assigns a string field of the Spec by its JSON name so form payloads can
// be applied without reflection.
func (s *Spec) set(key, value string) {
	switch key {
	case "name":
		s.Name = value
	case "server":
		s.Server = value
	case "port":
		s.Port, _ = strconv.Atoi(value)
	case "password":
		s.Password = value
	case "username":
		s.Username = value
	case "method":
		s.Method = value
	case "flow":
		s.Flow = value
	case "encryption":
		s.Encryption = value
	case "network":
		s.Network = value
	case "security":
		s.Security = value
	case "sni":
		s.SNI = value
	case "fingerprint":
		s.Fingerprint = value
	case "publicKey":
		s.PublicKey = value
	case "shortId":
		s.ShortID = value
	case "path":
		s.Path = value
	case "host":
		s.Host = value
	case "serviceName":
		s.ServiceName = value
	case "alpn":
		s.ALPN = value
	}
}

func cloneSpec(s Spec) Spec {
	b, _ := json.Marshal(s)
	var v Spec
	_ = json.Unmarshal(b, &v)
	return v
}
