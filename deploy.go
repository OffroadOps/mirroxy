package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

func (a *App) ensureCore(engine string) error {
	if engine != "xray" && engine != "sing-box" {
		return errors.New("不支持的核心")
	}
	if st, e := os.Stat(a.corePath(engine)); e == nil && st.Mode().IsRegular() && st.Size() > 0 {
		return nil
	}
	a.mu.Lock()
	enabled := a.cfg.AutoDownload
	a.mu.Unlock()
	if !enabled {
		return fmt.Errorf("%s 核心缺失，请开启自动下载或在全局设置安装", engine)
	}
	return a.updateCore(engine)
}

func (a *App) startNode(id string) error {
	a.mu.Lock()
	n := a.node(id)
	if n == nil {
		a.mu.Unlock()
		return errors.New("节点不存在")
	}
	engine := n.Engine
	a.mu.Unlock()
	if e := a.ensureCore(engine); e != nil {
		return e
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.changeNode(id, "start", "")
}

func (a *App) importAndDeploy(text, path string, deploy *bool) (M, error) {
	var nodes []Node
	var warnings []string
	var err error
	if path != "" {
		if serverPlatform() {
			return nil, errors.New("Linux 请粘贴分享链接或订阅地址导入")
		}
		nodes, warnings, err = importV2rayN(path)
	} else {
		nodes, warnings = parseLinks(text)
	}
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("没有导入有效节点：%s", strings.Join(warnings, "；"))
	}
	a.mu.Lock()
	before := cloneSettings(a.cfg)
	known := map[string]bool{}
	for _, n := range a.cfg.Nodes {
		known[n.ID] = true
	}
	count := a.importNodes(nodes, "手动导入")
	ids := []string{}
	for _, n := range a.cfg.Nodes {
		if !known[n.ID] {
			ids = append(ids, n.ID)
		}
	}
	shouldDeploy := a.cfg.AutoDeploy
	if deploy != nil {
		shouldDeploy = *deploy
	}
	if err = a.save(); err != nil {
		a.cfg = before
		a.mu.Unlock()
		return nil, err
	}
	a.event(fmt.Sprintf("已导入 %d 个节点", count))
	a.mu.Unlock()
	started := 0
	if shouldDeploy && len(ids) > 0 {
		var errs []string
		started, errs = a.deployNodes(ids)
		warnings = append(warnings, errs...)
	}
	return M{"count": count, "deployed": started, "nodeIds": ids, "warnings": warnings}, nil
}

// Downloads happen outside App.mu. Imported nodes remain saved even if a
// network download fails, so the user can retry Start without importing again.
func (a *App) deployNodes(ids []string) (int, []string) {
	a.mu.Lock()
	engines := map[string]bool{}
	for _, id := range ids {
		if n := a.node(id); n != nil {
			engines[n.Engine] = true
		}
	}
	a.mu.Unlock()
	errs := []string{}
	ready := map[string]bool{}
	for engine := range engines {
		if e := a.ensureCore(engine); e != nil {
			errs = append(errs, engine+" 部署失败："+e.Error())
		} else {
			ready[engine] = true
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.busyUpdate {
		return 0, append(errs, "核心正在更新，节点已保存，请稍后点击启动")
	}
	before := cloneSettings(a.cfg)
	started := 0
	for _, id := range ids {
		n := a.node(id)
		if n == nil || !ready[n.Engine] || n.Enabled {
			continue
		}
		if n.Port == 0 {
			p, e := a.allocatePort()
			if e != nil {
				errs = append(errs, e.Error())
				continue
			}
			n.Port = p
		} else if !portAvailable(n.Port) {
			errs = append(errs, fmt.Sprintf("%s 的端口 %d 已被占用", n.Name, n.Port))
			continue
		}
		if n.AppPort > 0 && !portAvailable(n.AppPort) {
			n.AppPort = 0
			p, e := a.allocateAppPort()
			if e != nil {
				errs = append(errs, e.Error())
				continue
			}
			n.AppPort = p
		}
		n.Enabled = true
		started++
	}
	if started == 0 {
		return 0, errs
	}
	err := a.reconcile()
	if err == nil {
		err = a.save()
	}
	if err != nil {
		a.cfg = before
		_ = a.reconcile()
		return 0, append(errs, "自动部署失败，节点已保存："+err.Error())
	}
	a.event(fmt.Sprintf("已自动部署 %d 个节点", started))
	return started, errs
}

func (a *App) recoverMissingCores() {
	a.mu.Lock()
	engines := map[string]bool{}
	for _, n := range a.cfg.Nodes {
		if n.Enabled {
			engines[n.Engine] = true
		}
	}
	a.mu.Unlock()
	for engine := range engines {
		if _, e := os.Stat(a.corePath(engine)); e == nil {
			continue
		}
		if e := a.ensureCore(engine); e != nil {
			a.mu.Lock()
			a.event(coreDisplayName(engine) + " 自动安装失败：" + e.Error())
			a.mu.Unlock()
		}
	}
}

func (a *App) refreshSubscription(id string) (int, error) {
	result, err := a.refreshSubscriptionAndDeploy(id)
	if err != nil {
		return 0, err
	}
	return result["count"].(int), nil
}

func (a *App) refreshSubscriptionAndDeploy(id string) (M, error) {
	a.mu.Lock()
	known := map[string]bool{}
	for _, n := range a.cfg.Nodes {
		known[n.ID] = true
	}
	a.mu.Unlock()
	count, err := a.refreshSubscriptionStored(id)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	ids := []string{}
	if a.cfg.AutoDeploy {
		for _, n := range a.cfg.Nodes {
			if n.Source == id && !known[n.ID] {
				ids = append(ids, n.ID)
			}
		}
	}
	a.mu.Unlock()
	started, warnings := 0, []string{}
	if len(ids) > 0 {
		started, warnings = a.deployNodes(ids)
	}
	return M{"count": count, "deployed": started, "warnings": warnings}, nil
}
