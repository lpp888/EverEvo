//go:build windows

package main

import (
	"os"
	"path/filepath"

	"everevo/internal/tools"
)

func init() {
	// Register all app-control handlers in the global map.
	// These give the LLM full control over every app feature.

	toolHandlers["memory_list"] = hMemoryList
	toolHandlers["memory_search"] = hMemorySearch
	toolHandlers["memory_delete"] = hMemoryDelete
	toolHandlers["memory_add_fact"] = hMemoryAddFact
	toolHandlers["memory_clear"] = hMemoryClear
	toolHandlers["core_list"] = hCoreList
	toolHandlers["core_add"] = hCoreAdd
	toolHandlers["core_delete"] = hCoreDelete
	toolHandlers["session_list"] = hSessionList
	toolHandlers["session_delete"] = hSessionDelete
	toolHandlers["graph_list"] = hGraphList
	toolHandlers["graph_add_edge"] = hGraphAddEdge
	toolHandlers["graph_delete_node"] = hGraphDeleteNode
	toolHandlers["graph_rename_node"] = hGraphRenameNode
	toolHandlers["wiki_list"] = hWikiList
	toolHandlers["wiki_read"] = hWikiRead
	toolHandlers["wiki_save"] = hWikiSave
	toolHandlers["wiki_delete"] = hWikiDelete
	toolHandlers["wiki_search"] = hWikiSearch
	toolHandlers["wiki_reindex"] = hWikiReindex
	toolHandlers["experience_list"] = hExperienceList
	toolHandlers["experience_delete"] = hExperienceDelete
	toolHandlers["library_list"] = hLibraryList2
	toolHandlers["library_create"] = hLibraryCreate2
	toolHandlers["library_delete"] = hLibraryDelete2
	toolHandlers["write_file"] = hWriteFile
	toolHandlers["list_directory"] = hListDirectory
}

// ── Memory ──

func hMemoryList(a *App, p map[string]any) tools.ToolResult {
	limit := tools.GetInt(p, "limit")
	if limit <= 0 { limit = 20 }
	list, err := a.MemoryList(limit)
	if err != nil { return tools.ErrResult(err) }
	return tools.OkResult(list)
}

func hMemorySearch(a *App, p map[string]any) tools.ToolResult {
	query := tools.GetString(p, "query")
	k := tools.GetInt(p, "k")
	if k <= 0 { k = 5 }
	result, err := a.MemoryRecall(query, k)
	if err != nil { return tools.ErrResult(err) }
	return tools.OkResult(result)
}

func hMemoryDelete(a *App, p map[string]any) tools.ToolResult {
	id := tools.GetString(p, "id")
	if id == "" { return tools.ErrMsg("id required") }
	if err := a.MemoryItemDelete(id); err != nil { return tools.ErrResult(err) }
	return tools.OkResult("deleted")
}

func hMemoryAddFact(a *App, p map[string]any) tools.ToolResult {
	content := tools.GetString(p, "content")
	category := tools.GetString(p, "category")
	if content == "" || category == "" { return tools.ErrMsg("content and category required") }
	if err := a.MemoryCoreAdd(category, content, category); err != nil { return tools.ErrResult(err) }
	return tools.OkResult("fact added")
}

func hMemoryClear(a *App, p map[string]any) tools.ToolResult {
	if err := a.MemoryClear(); err != nil { return tools.ErrResult(err) }
	return tools.OkResult("cleared")
}

// ── Core Facts ──

func hCoreList(a *App, p map[string]any) tools.ToolResult {
	facts, err := a.MemoryCoreList()
	if err != nil { return tools.ErrResult(err) }
	return tools.OkResult(facts)
}

func hCoreAdd(a *App, p map[string]any) tools.ToolResult {
	key := tools.GetString(p, "key")
	value := tools.GetString(p, "value")
	category := tools.GetString(p, "category")
	if key == "" { key = category }
	if value == "" { return tools.ErrMsg("value required") }
	if err := a.MemoryCoreAdd(key, value, category); err != nil { return tools.ErrResult(err) }
	return tools.OkResult("core fact added")
}

func hCoreDelete(a *App, p map[string]any) tools.ToolResult {
	id := tools.GetString(p, "id")
	if id == "" { return tools.ErrMsg("id required") }
	if err := a.MemoryCoreDelete(id); err != nil { return tools.ErrResult(err) }
	return tools.OkResult("deleted")
}

// ── Sessions ──

func hSessionList(a *App, p map[string]any) tools.ToolResult {
	list, err := a.MemorySessionList()
	if err != nil { return tools.ErrResult(err) }
	return tools.OkResult(list)
}

func hSessionDelete(a *App, p map[string]any) tools.ToolResult {
	id := tools.GetString(p, "id")
	if id == "" { return tools.ErrMsg("id required") }
	if err := a.MemorySessionDelete(id); err != nil { return tools.ErrResult(err) }
	return tools.OkResult("deleted")
}

// ── Knowledge Graph ──

func hGraphList(a *App, p map[string]any) tools.ToolResult {
	search := tools.GetString(p, "search")
	libID, _ := a.memoryStore.DefaultLibrary()
	result, err := a.MemoryGraphList(false, libID)
	if err != nil { return tools.ErrResult(err) }
	nodes, _ := result["nodes"].([]any)
	if search != "" {
		var filtered []any
		for _, n := range nodes {
			if nm, ok := n.(map[string]any); ok {
				name, _ := nm["name"].(string)
				if contains(name, search) { filtered = append(filtered, n) }
			}
		}
		nodes = filtered
	}
	return tools.OkResult(map[string]any{"nodes": nodes, "edges": result["edges"]})
}

func hGraphAddEdge(a *App, p map[string]any) tools.ToolResult {
	srcName := tools.GetString(p, "srcName")
	dstName := tools.GetString(p, "dstName")
	relType := tools.GetString(p, "type")
	if srcName == "" || dstName == "" || relType == "" { return tools.ErrMsg("srcName, dstName, type required") }
	replaces := false
	if v, ok := p["replaces"].(bool); ok { replaces = v }
	if err := a.MemoryEdgeAdd(srcName, dstName, relType, replaces); err != nil { return tools.ErrResult(err) }
	return tools.OkResult("edge added")
}

func hGraphDeleteNode(a *App, p map[string]any) tools.ToolResult {
	id := tools.GetString(p, "id")
	if id == "" { return tools.ErrMsg("id required") }
	if err := a.MemoryNodeDelete(id); err != nil { return tools.ErrResult(err) }
	return tools.OkResult("deleted")
}

func hGraphRenameNode(a *App, p map[string]any) tools.ToolResult {
	id := tools.GetString(p, "id")
	name := tools.GetString(p, "name")
	if id == "" || name == "" { return tools.ErrMsg("id and name required") }
	if err := a.MemoryNodeRename(id, name); err != nil { return tools.ErrResult(err) }
	return tools.OkResult("renamed")
}

// ── Wiki ──

func hWikiList(a *App, p map[string]any) tools.ToolResult {
	libID, _ := a.memoryStore.DefaultLibrary()
	pages, err := a.WikiListPages(libID)
	if err != nil { return tools.ErrResult(err) }
	return tools.OkResult(pages)
}

func hWikiRead(a *App, p map[string]any) tools.ToolResult {
	pageID := tools.GetString(p, "pageId")
	if pageID == "" { return tools.ErrMsg("pageId required") }
	libID, _ := a.memoryStore.DefaultLibrary()
	content, err := a.WikiReadPage(libID, pageID)
	if err != nil { return tools.ErrResult(err) }
	return tools.OkResult(content)
}

func hWikiSave(a *App, p map[string]any) tools.ToolResult {
	pageID := tools.GetString(p, "pageId")
	title := tools.GetString(p, "title")
	content := tools.GetString(p, "content")
	if title == "" || content == "" { return tools.ErrMsg("title and content required") }
	if pageID == "" { pageID = title }
	libID, _ := a.memoryStore.DefaultLibrary()
	if err := a.WikiSavePage(libID, pageID, title, content); err != nil { return tools.ErrResult(err) }
	return tools.OkResult(map[string]string{"pageId": pageID, "status": "saved"})
}

func hWikiDelete(a *App, p map[string]any) tools.ToolResult {
	pageID := tools.GetString(p, "pageId")
	if pageID == "" { return tools.ErrMsg("pageId required") }
	libID, _ := a.memoryStore.DefaultLibrary()
	if err := a.WikiDeletePage(libID, pageID); err != nil { return tools.ErrResult(err) }
	return tools.OkResult("deleted")
}

func hWikiSearch(a *App, p map[string]any) tools.ToolResult {
	query := tools.GetString(p, "query")
	if query == "" { return tools.ErrMsg("query required") }
	libID, _ := a.memoryStore.DefaultLibrary()
	hits, err := a.WikiSearch(libID, query)
	if err != nil { return tools.ErrResult(err) }
	return tools.OkResult(hits)
}

func hWikiReindex(a *App, p map[string]any) tools.ToolResult {
	libID, _ := a.memoryStore.DefaultLibrary()
	result, err := a.WikiReindex(libID)
	if err != nil { return tools.ErrResult(err) }
	return tools.OkResult(result)
}

// ── Experience ──

func hExperienceList(a *App, p map[string]any) tools.ToolResult {
	limit := tools.GetInt(p, "limit")
	if limit <= 0 { limit = 10 }
	libID, _ := a.memoryStore.DefaultLibrary()
	items, err := a.MemoryRecallExperience(libID, limit)
	if err != nil { return tools.ErrResult(err) }
	return tools.OkResult(items)
}

func hExperienceDelete(a *App, p map[string]any) tools.ToolResult {
	id := tools.GetString(p, "id")
	if id == "" { return tools.ErrMsg("id required") }
	if err := a.MemoryExperienceDelete(id); err != nil { return tools.ErrResult(err) }
	return tools.OkResult("deleted")
}

// ── Domain Libraries ──

func hLibraryList2(a *App, p map[string]any) tools.ToolResult {
	list, err := a.LibraryList()
	if err != nil { return tools.ErrResult(err) }
	return tools.OkResult(list)
}

func hLibraryCreate2(a *App, p map[string]any) tools.ToolResult {
	name := tools.GetString(p, "name")
	desc := tools.GetString(p, "description")
	if name == "" { return tools.ErrMsg("name required") }
	id, err := a.LibraryCreate(name, desc, false)
	if err != nil { return tools.ErrResult(err) }
	return tools.OkResult(map[string]string{"id": id, "name": name})
}

func hLibraryDelete2(a *App, p map[string]any) tools.ToolResult {
	id := tools.GetString(p, "id")
	if id == "" { return tools.ErrMsg("id required") }
	if err := a.LibraryDelete(id); err != nil { return tools.ErrResult(err) }
	return tools.OkResult("deleted")
}

// ── File write ──

func hWriteFile(a *App, p map[string]any) tools.ToolResult {
	path := tools.GetString(p, "path")
	content := tools.GetString(p, "content")
	if path == "" || content == "" { return tools.ErrMsg("path and content required") }
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil { return tools.ErrResult(err) }
	if err := os.WriteFile(path, []byte(content), 0644); err != nil { return tools.ErrResult(err) }
	return tools.OkResult(map[string]any{"path": path, "size": len(content)})
}

func hListDirectory(a *App, p map[string]any) tools.ToolResult {
	path := tools.GetString(p, "path")
	if path == "" { return tools.ErrMsg("path required") }
	entries, err := os.ReadDir(path)
	if err != nil { return tools.ErrResult(err) }
	var out []map[string]any
	for _, e := range entries {
		out = append(out, map[string]any{"name": e.Name(), "isDir": e.IsDir()})
	}
	return tools.OkResult(map[string]any{"entries": out})
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) > 0 && findSub(s, sub))
}
func findSub(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub { return true }
	}
	return false
}
