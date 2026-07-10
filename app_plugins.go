//go:build windows

package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"everevo/internal/plugin"
	"everevo/internal/storage"
)

// ─── 插件 API ────────────────────────────────────────────────

var pluginHost *plugin.Host
var pluginClient *plugin.Client
var pluginMu sync.Mutex

func (a *App) getPluginHost() *plugin.Host {
	pluginMu.Lock()
	defer pluginMu.Unlock()
	if pluginHost == nil {
		dir, _ := storage.DataDir()
		pluginHost = plugin.NewHost(dir)
		pluginClient = plugin.NewClient(pluginHost)
	}
	return pluginHost
}

// ListPlugins 扫描并返回所有已安装插件。
func (a *App) ListPlugins() ([]plugin.Spec, error) {
	dir, err := storage.DataDir()
	if err != nil {
		return nil, err
	}
	specs, err := plugin.ScanPlugins(plugin.PluginsDir(dir))
	if err != nil {
		return nil, err
	}
	if specs == nil {
		specs = []plugin.Spec{}
	}
	return specs, nil
}

// GetPluginStatus 返回插件的运行状态。
func (a *App) GetPluginStatus(name string) plugin.Status {
	host := a.getPluginHost()
	return host.GetStatus(name)
}

// StartPlugin 启动指定插件（会自动启动 stdout 读取协程）。
func (a *App) StartPlugin(name string) error {
	dir, _ := storage.DataDir()
	specs, _ := plugin.ScanPlugins(plugin.PluginsDir(dir))
	spec, err := plugin.Lookup(specs, name)
	if err != nil {
		return err
	}
	host := a.getPluginHost()
	if err := host.Start(*spec); err != nil {
		return err
	}
	// 启动后台读取协程以接收 RPC 响应
	pluginClient.StartReader(name)
	// 健康检查（非致命）
	if err := pluginClient.Health(name); err != nil {
		log.Printf("[plugin] %s 健康检查失败: %v", name, err)
	}
	a.emitChanged("plugins:changed", "update", name)
	return nil
}

// StopPlugin 停止指定插件。
func (a *App) StopPlugin(name string) error {
	if err := a.getPluginHost().Stop(name); err != nil {
		return err
	}
	a.emitChanged("plugins:changed", "update", name)
	return nil
}

// RestartPlugin 重启指定插件。
func (a *App) RestartPlugin(name string) error {
	if err := a.getPluginHost().Restart(name); err != nil {
		return err
	}
	a.emitChanged("plugins:changed", "update", name)
	return nil
}

// RunPlugin 调用插件方法。
func (a *App) RunPlugin(name, method string, params map[string]any) (map[string]any, error) {
	return pluginClient.Call(name, method, params, 30*time.Second)
}

// PickPluginFile 打开文件选择对话框，选 .zip 插件包。
func (a *App) PickPluginFile() string {
	path, _ := pickPluginDialog()
	return path
}

// InstallPlugin 从给定路径安装插件（支持 .zip 和目录）。
func (a *App) InstallPlugin(path string) (plugin.Spec, error) {
	dataDir, err := storage.DataDir()
	if err != nil {
		return plugin.Spec{}, err
	}
	pluginsDir := plugin.PluginsDir(dataDir)
	tmpDir := plugin.TmpDir(dataDir)
	os.MkdirAll(tmpDir, 0755)

	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".zip" {
		spec, err := plugin.InstallFromZip(path, pluginsDir, tmpDir)
		if err != nil {
			return plugin.Spec{}, err
		}
		log.Printf("[plugin] 已安装: %s v%s (from %s)", spec.Name, spec.Version, filepath.Base(path))
		a.emitChanged("plugins:changed", "update", spec.Name)
		return *spec, nil
	}
	spec, err := plugin.InstallFromDir(path, pluginsDir)
	if err != nil {
		return plugin.Spec{}, err
	}
	log.Printf("[plugin] 已安装: %s v%s (from %s)", spec.Name, spec.Version, filepath.Base(path))
	a.emitChanged("plugins:changed", "update", spec.Name)
	return *spec, nil
}

// DeletePlugin 卸载指定插件（先停止再删除目录）。
func (a *App) DeletePlugin(name string) error {
	// Stop first if running
	host := a.getPluginHost()
	if host.IsRunning(name) {
		if err := host.Stop(name); err != nil {
			log.Printf("[plugin] 停止 %s 失败: %v", name, err)
		}
	}
	dataDir, err := storage.DataDir()
	if err != nil {
		return err
	}
	pluginsDir := plugin.PluginsDir(dataDir)
	if err := plugin.DeletePlugin(pluginsDir, name); err != nil {
		return err
	}
	log.Printf("[plugin] 已卸载: %s", name)
	a.emitChanged("plugins:changed", "update", name)
	return nil
}

// GetPluginLogs 返回插件最近的 stderr 日志（最多 64KB）。
func (a *App) GetPluginLogs(name string) string {
	return a.getPluginHost().GetLogs(name)
}
