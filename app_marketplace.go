//go:build windows

package main

import (
	"everevo/internal/marketplace"
	mcpclient "everevo/internal/mcp/client"
	"everevo/internal/skills"
)

// ─── Skill Marketplace API ───────────────────────────────────────

// ListMarketSkills returns the built-in marketplace with install status.
func (a *App) ListMarketSkills() []marketplace.SkillPackage {
	pkgs, _ := marketplace.FetchMarket()
	// Only show real market data — no fallback to fake built-in content
	if pkgs == nil {
		pkgs = []marketplace.SkillPackage{}
	}
	installed := a.skillManager.List()
	installedNames := map[string]bool{}
	for _, s := range installed {
		installedNames[s.Name] = true
	}
	for i := range pkgs {
		pkgs[i].Installed = installedNames[pkgs[i].Name]
	}
	return pkgs
}

// RefreshMarketSkills forces a re-fetch from the online source.
func (a *App) RefreshMarketSkills() ([]marketplace.SkillPackage, error) {
	pkgs, err := marketplace.RefreshMarket()
	if err != nil {
		return nil, err
	}
	// Merge install status
	installed := a.skillManager.List()
	installedNames := map[string]bool{}
	for _, s := range installed {
		installedNames[s.Name] = true
	}
	for i := range pkgs {
		pkgs[i].Installed = installedNames[pkgs[i].Name]
	}
	return pkgs, nil
}

// InstallMarketSkill installs a skill from the marketplace (auto-adds deps).
func (a *App) InstallMarketSkill(pkg marketplace.SkillPackage) (marketplace.InstallResult, error) {
	result := marketplace.InstallResult{SkillName: pkg.Name}

	// Check if skill already exists
	existing := a.skillManager.List()
	for _, s := range existing {
		if s.Name == pkg.Name {
			result.Existing = true
		}
	}

	// Add dependent MCP servers
	for _, dep := range pkg.MCPServers {
		cfg := mcpclient.ServerConfig{
			ID: "srv_" + dep.Name, Name: dep.Name,
			Transport: dep.Transport, Command: dep.Command,
			Args: dep.Args, URL: dep.URL,
		}
		if err := a.mcpClient.AddServer(cfg); err != nil {
			// Already exists — skip
		} else {
			result.MCPServers = append(result.MCPServers, dep.Name)
			// Try to connect
			go a.mcpClient.Connect(cfg.ID)
		}
	}

	// Build skill
	sk := skills.Skill{
		Name:         pkg.Name,
		Title:        pkg.Title,
		Description:  pkg.Description,
		Category:     pkg.Category,
		Icon:         pkg.Icon,
		Enabled:      true,
		Tools:        pkg.Tools,
		MCPTools:     pkg.MCPTools,
		SystemPrompt: pkg.SystemPrompt,
	}

	if result.Existing {
		if err := a.skillManager.Update(pkg.Name, sk); err != nil {
			return result, err
		}
	} else {
		if err := a.skillManager.Create(sk); err != nil {
			return result, err
		}
	}
	if err := a.skillManager.Save(); err != nil {
		return result, err
	}
	return result, nil
}

// UninstallMarketSkill removes a marketplace skill (keeps MCP servers).
func (a *App) UninstallMarketSkill(name string) error {
	return a.skillManager.Delete(name)
}
