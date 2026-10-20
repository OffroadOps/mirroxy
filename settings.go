package main

import "errors"

type SettingsPatch struct {
	StartPort                     *int    `json:"startPort"`
	AutoUpdate                    *bool   `json:"autoUpdate"`
	AutoSubscribe                 *bool   `json:"autoSubscribe"`
	Autostart                     *bool   `json:"autostart"`
	OpenBrowser                   *bool   `json:"openBrowser"`
	ResumeNodes                   *bool   `json:"resumeNodes"`
	DefaultRoute                  *string `json:"defaultRoute"`
	LogLevel                      *string `json:"logLevel"`
	SubscribeHours                *int    `json:"subscribeHours"`
	HealthMonitor                 *bool   `json:"healthMonitor"`
	HealthInterval                *int    `json:"healthInterval"`
	AutoDownload                  *bool   `json:"autoDownload"`
	AutoDeploy                    *bool   `json:"autoDeploy"`
	PauseSystemProxyOnAccelerator *bool   `json:"pauseSystemProxyOnAccelerator"`
	BypassLAN                     *bool   `json:"bypassLAN"`
	GeoAutoUpdate                 *bool   `json:"geoAutoUpdate"`
	PanelPort                     *int    `json:"panelPort"`
}

func (a *App) saveSettings(p SettingsPatch) error {
	before := cloneSettings(a.cfg)
	if p.AutoDownload != nil {
		a.cfg.AutoDownload = *p.AutoDownload
	}
	if p.AutoDeploy != nil {
		a.cfg.AutoDeploy = *p.AutoDeploy
	}
	if p.PauseSystemProxyOnAccelerator != nil {
		a.cfg.PauseSystemProxyOnAccelerator = p.PauseSystemProxyOnAccelerator
	}
	if p.BypassLAN != nil {
		a.cfg.BypassLAN = p.BypassLAN
	}
	if p.GeoAutoUpdate != nil {
		a.cfg.GeoAutoUpdate = *p.GeoAutoUpdate
	}
	panelPortChanged := 0
	if p.PanelPort != nil && *p.PanelPort != a.configuredPanelPort() {
		if *p.PanelPort < 1024 || *p.PanelPort > 65000 {
			a.cfg = before
			return errors.New("面板端口必须在 1024–65000")
		}
		a.cfg.PanelPort = *p.PanelPort
		panelPortChanged = *p.PanelPort
	}
	if p.StartPort != nil {
		a.cfg.StartPort = *p.StartPort
	}
	if p.AutoUpdate != nil {
		a.cfg.AutoUpdate = *p.AutoUpdate
	}
	if p.AutoSubscribe != nil {
		a.cfg.AutoSubscribe = *p.AutoSubscribe
	}
	if p.Autostart != nil {
		a.cfg.Autostart = *p.Autostart
	}
	if p.OpenBrowser != nil {
		a.cfg.OpenBrowser = *p.OpenBrowser
	}
	if p.ResumeNodes != nil {
		a.cfg.ResumeNodes = *p.ResumeNodes
	}
	if p.DefaultRoute != nil {
		a.cfg.DefaultRoute = *p.DefaultRoute
	}
	if p.LogLevel != nil {
		a.cfg.LogLevel = *p.LogLevel
	}
	if p.SubscribeHours != nil {
		a.cfg.SubscribeHours = *p.SubscribeHours
	}
	if p.HealthMonitor != nil {
		a.cfg.HealthMonitor = *p.HealthMonitor
	}
	if p.HealthInterval != nil {
		a.cfg.HealthInterval = *p.HealthInterval
	}
	var err error
	switch {
	case a.cfg.StartPort < 1024 || a.cfg.StartPort > 65000:
		err = errors.New("起始端口必须在 1024–65000")
	case a.cfg.SubscribeHours < 1 || a.cfg.SubscribeHours > 168:
		err = errors.New("订阅更新间隔必须在 1–168 小时")
	case a.cfg.HealthInterval < 30 || a.cfg.HealthInterval > 3600:
		err = errors.New("可用性检测间隔必须在 30–3600 秒")
	case !validNodeRoute(a.cfg.DefaultRoute):
		err = errors.New("无效默认出口策略")
	case a.cfg.LogLevel != "warning" && a.cfg.LogLevel != "error" && a.cfg.LogLevel != "info" && a.cfg.LogLevel != "debug":
		err = errors.New("无效日志等级")
	}
	if err != nil {
		a.cfg = before
		return err
	}
	if a.cfg.Autostart != before.Autostart {
		err = a.setAutostart(a.cfg.Autostart)
	}
	if err == nil {
		err = a.reconcile()
	}
	if err == nil {
		err = a.save()
	}
	if err != nil {
		autostartChanged := a.cfg.Autostart != before.Autostart
		a.cfg = before
		if autostartChanged {
			_ = a.setAutostart(before.Autostart)
		}
		_ = a.reconcile()
		return err
	}
	a.event("全局设置已保存")
	if panelPortChanged > 0 {
		go func() {
			if err := a.rebindPanelPort(panelPortChanged); err != nil {
				a.mu.Lock()
				a.cfg.PanelPort = before.PanelPort
				_ = a.save()
				a.event("面板端口切换失败，已保持原端口：" + err.Error())
				a.mu.Unlock()
			}
		}()
	}
	return nil
}
