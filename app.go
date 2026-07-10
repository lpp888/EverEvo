//go:build windows

package main

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"

	"everevo/internal/backends"
	"everevo/internal/backends/llama"
	"everevo/internal/backends/onnx"
	"everevo/internal/catalog"
	"everevo/internal/config"
	"everevo/internal/downloader"
	"everevo/internal/guides"
	"everevo/internal/httpclient"
	"everevo/internal/memory"
	"everevo/internal/model"
	"everevo/internal/storage"
	"everevo/internal/sysinfo"
	"everevo/internal/wiki"

	"everevo/internal/a2a"
	"everevo/internal/agents"
	"everevo/internal/feishu"
	"everevo/internal/mcp"
	mcpclient "everevo/internal/mcp/client"
	"everevo/internal/skills"
	"everevo/internal/tools"
	"everevo/internal/workflow"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App 是 Wails 应用结构体。所有公开方法自动暴露给前端。
type App struct {
	ctx             context.Context
	chatCtx         context.Context
	chatCancel      context.CancelFunc
	cfg             *config.Config
	manager         *model.Manager
	sysInfoCache    *sysinfo.SysInfo
	dlManager       *downloader.Manager
	mcpServer       *mcp.Server
	mcpClient       *mcpclient.Manager
	a2aManager      *a2a.Manager
	feishuClient    *feishu.Client
	skillManager    *skills.Manager
	agentManager    *agents.Manager
	memoryStore     *memory.Store
	guideManager    *guides.Manager
	workflowManager *workflow.Manager
	memSweepDone    chan struct{}
	wikiStores      map[string]*wiki.Store           // per-library wiki stores, keyed by libraryID
	wikiStoreMu     sync.RWMutex
	streamCancelMu  sync.Mutex
	streamCancels   map[string]context.CancelFunc    // streamID → cancel
}

// NewApp 创建应用实例。
func NewApp() *App {
	return &App{}
}

// initWindowSize sizes and centers the window at 72% × 78% of the primary
// screen's logical dimensions. Respects MinWidth/MinHeight and never exceeds
// the physical screen bounds.
func initWindowSize(ctx context.Context) {
	screens, err := wailsRuntime.ScreenGetAll(ctx)
	if err != nil || len(screens) == 0 {
		return
	}

	// Use first screen (primary on single-monitor, usually primary on multi).
	s := screens[0]

	const (
		widthRatio  = 0.72
		heightRatio = 0.78
	)

	w := int(float64(s.Size.Width) * widthRatio)
	h := int(float64(s.Size.Height) * heightRatio)

	// Clamp to min/max bounds.
	if w < 640 {
		w = 640
	}
	if h < 480 {
		h = 480
	}
	if w > s.Size.Width {
		w = s.Size.Width
	}
	if h > s.Size.Height {
		h = s.Size.Height
	}

	wailsRuntime.WindowSetSize(ctx, w, h)
	wailsRuntime.WindowCenter(ctx)
}

// startup 在 Wails 应用启动时调用。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.chatCtx, a.chatCancel = context.WithCancel(context.Background())

	// ── Adaptive window sizing ─────────────────────────────────
	// Default size is 72% × 78% of the primary screen's work area,
	// constrained by MinWidth/MinHeight and physical screen bounds.
	initWindowSize(ctx)

	// 日志同时写到文件和终端（dev 模式下终端可见，生产 EXE 只看文件）
	storage.EnsureDataDir()
	dataDir, _ := storage.AppDataDir()
	logPath := filepath.Join(dataDir, "EverEvo.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	var logW io.Writer = os.Stdout
	if err == nil {
		logW = io.MultiWriter(os.Stdout, logFile)
	}
	log.SetOutput(logW)
	log.SetFlags(log.Ltime | log.Lmsgprefix)
	log.SetPrefix("[EverEvo] ")
	log.Printf("日志文件: %s", logPath)
	log.Println("══════════━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println("应用启动")

	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println("应用启动中……")

	cfg, err := config.Load()
	if err != nil {
		log.Printf("⚠ 加载用户配置失败: %v，使用默认配置", err)
		cfg = config.Defaults()
	}
	a.cfg = cfg
	log.Printf("用户配置: %s", config.Path())
	// Apply proxy config
	httpclient.SetUserProxy(cfg.LLM.HTTPProxy)
	if cfg.LLM.ProxyEnabled != nil && !*cfg.LLM.ProxyEnabled {
		httpclient.SetEnabled(false)
		log.Printf("ℹ 代理已禁用（用户设置）")
	}
	if ps := httpclient.Detect(); ps.Source != "none" {
		log.Printf("✓ 网络代理: %s (来源: %s)", ps.URL, ps.Source)
	}
	// Async proxy health check (logs warning if unreachable)
	go httpclient.HealthCheck()
	// 加载已存凭证到 catalog
	for src, tok := range cfg.Credentials {
		catalog.SetCredential(src, catalog.Credential{Token: tok})
		log.Printf("已加载 %s 凭证", src)
	}

	log.Println("运行模式: 便携版")

	if err := storage.EnsureDataDir(); err != nil {
		log.Printf("⚠ 创建数据目录失败: %v", err)
	} else {
		dir, _ := storage.DataDir()
		modelsDir, _ := storage.ModelsDir()
		log.Printf("数据目录: %s", dir)
		log.Printf("模型目录: %s", modelsDir)
	}

	// 尝试初始化 ONNX Runtime（如果已安装）
	onnxDLL, _ := findONNXDLL()
	if onnxDLL != "" {
		if err := onnx.Init(onnxDLL); err != nil {
			log.Printf("⚠ ONNX Runtime 初始化失败: %v (DLL: %s)", err, onnxDLL)
		} else {
			log.Printf("✓ ONNX Runtime 就绪 (%s)", filepath.Base(onnxDLL))
		}
	} else {
		log.Println("ℹ 未找到 ONNX Runtime DLL，推理功能不可用")
	}

	// 尝试初始化 llama.cpp（best-effort，通过 llama-server.exe 子进程）
	llamaBin, _ := findLlamaDLL()
	if llamaBin != "" {
		if err := llama.Init(llamaBin); err != nil {
			log.Printf("⚠ llama.cpp 初始化失败: %v", err)
		} else {
			log.Printf("✓ llama.cpp 就绪 (%s)", filepath.Base(llamaBin))
		}
	} else {
		log.Println("ℹ 未找到 llama-server.exe，GGUF 模型将无法推理")
		log.Println("   下载: https://github.com/ggml-org/llama.cpp/releases")
	}

	a.manager = model.NewManager()
	a.dlManager = downloader.NewManager(func(event string, data interface{}) {
		wailsRuntime.EventsEmit(a.ctx, event, data)
	})
	// Persist download history across restarts.
	if dataDir, err := storage.AppDataDir(); err == nil {
		historyPath := filepath.Join(dataDir, "download_history.json")
		a.dlManager.SetHistoryPath(historyPath)
		a.dlManager.LoadHistory()
		log.Printf("下载历史: %s (%d 条记录)", historyPath, len(a.dlManager.History()))
	}
	log.Println("模型管理器就绪")

	// Register all LLM-callable tools
	tools.RegisterAll()
	log.Printf("已注册 %d 个 LLM 工具", len(tools.List()))

	// Initialize skill manager
	a.skillManager = skills.NewManager()
	if err := a.skillManager.Save(); err != nil {
		log.Printf("[skills] 初次持久化失败: %v", err)
	}
	log.Printf("已加载 %d 个能力域 (Skill)", len(a.skillManager.List()))

	// Initialize local agent manager (personas) — ensures a default main agent exists
	a.agentManager = agents.NewManager()
	log.Printf("已加载 %d 个本地 Agent", len(a.agentManager.List()))

	// Initialize MCP Client — auto-connect external MCP servers
	a.mcpClient = mcpclient.NewManager()
	a.mcpClient.LoadAndConnect()

	// Initialize A2A Agent Manager — server + client for agent-to-agent communication
	a.initA2AManager()
	log.Println("A2A Agent 管理器就绪")

	// Initialize Feishu bot (WebSocket long-connection to Feishu)
	a.initFeishuClient()
	log.Println("飞书机器人就绪")

	// Initialize guide manager
	a.guideManager = guides.NewManager()
	log.Printf("已加载 %d 个攻略来源", len(a.guideManager.ListSources()))

	// Initialize workflow manager
	a.workflowManager = workflow.NewManager()
	log.Printf("已加载 %d 个工作流", len(a.workflowManager.List()))

	// Initialize memory store (conversation persistence; the temporal knowledge
	// graph arrives in P2 using the same SQLite handle).
	a.memoryStore, err = memory.NewStore()
	if err != nil {
		log.Printf("⚠ 记忆数据库初始化失败: %v", err)
	} else {
		log.Println("记忆数据库就绪")
		// P1: bind an embedding model for long-term semantic memory. Auto-detect
		// the first sentence-embedding model under data/models; degrade silently
		// (recall stays empty) if none is installed.
		if a.memoryStore.EmbeddingModelDir() == "" {
			if dir := detectEmbeddingModelDir(); dir != "" {
				if e := a.memoryStore.SetEmbeddingModel(dir); e == nil {
					log.Printf("记忆向量模型: %s", filepath.Base(dir))
				}
			} else {
				log.Println("ℹ 未找到句向量模型，长期记忆检索将关闭（下载 sentence-transformers 模型即可启用）")
			}
		}
	}

	// P5: compute hardware-adaptive memory policy (RAM/disk → tier → params).
	a.applyMemoryPolicy()
	// P5: boot + daily TTL sweep of expired episodic memory (core layer untouched).
	a.memSweepDone = make(chan struct{})
	go a.runMemorySweep()

	// P7: seed default workspace + domain library
	if a.memoryStore != nil {
		_, _ = a.memoryStore.DefaultWorkspace()
		libID, _ := a.memoryStore.DefaultLibrary()
		// Backfill legacy agents with the default library ID.
		if libID != "" && a.agentManager != nil {
			_ = a.agentManager.EnsureLibraryIDs(libID)
		}
			// Migrate legacy 'default' workspace_id to actual library ID.
			if libID != "" && a.memoryStore != nil {
			a.memoryStore.MigrateDefaultWorkspace(libID)
			}
	}

	// P6.1: wiki index — per-library, lazy-init. Core library indexes docs/ at boot.
	a.wikiStores = make(map[string]*wiki.Store)
	if coreLibID, _ := a.memoryStore.DefaultLibrary(); coreLibID != "" {
		if ws, wErr := wiki.NewStore(coreLibID); wErr == nil {
			a.wikiStores[coreLibID] = ws
			go func() { _, _ = a.WikiReindex(coreLibID) }()
		} else {
			log.Printf("⚠ wiki 索引初始化失败: %v", wErr)
		}
	}
	// P9: start dream pipeline scheduler (Light→REM→Deep).
	a.startDreamScheduler()


	// Start the internal MCP server (auto-started when port is configured).
	if a.cfg.LLM.MCPPort > 0 {
		a.StartMCPServer()
	}
}

// ─── Startup helpers ──────────────────────────────────────────────

// findONNXDLL searches for onnxruntime.dll using the unified backends scanner.
func findONNXDLL() (string, bool) {
	dll := backends.FindDLL("onnxruntime*.dll")
	return dll, dll != ""
}

// findLlamaDLL searches for llama-server.exe using the unified backends scanner.
func findLlamaDLL() (string, bool) {
	exe := backends.FindDLL("llama-server.exe")
	return exe, exe != ""
}

// detectEmbeddingModelDir returns the path of the first detected
// sentence-embedding model under the models directory.
func detectEmbeddingModelDir() string {
	candidates := []string{
		"sentence-transformers_all-MiniLM-L6-v2",
		"sentence-transformers_all-mpnet-base-v2",
		"sentence-transformers_paraphrase-multilingual-MiniLM-L12-v2",
	}
	// Search EXE dir + working dir for models.
	exe, _ := os.Executable()
	dirs := []string{filepath.Join(filepath.Dir(exe), "data", "models")}
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, filepath.Join(wd, "data", "models"), filepath.Join(wd, "build", "bin", "data", "models"))
	}
	if md, err := storage.ModelsDir(); err == nil {
		dirs = append(dirs, md)
	}
	for _, modelsDir := range dirs {
		for _, c := range candidates {
			p := filepath.Join(modelsDir, c)
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				return p
			}
		}
	}
	return ""
}

// ─── Knowledge Graph — P2 view (frontend graph viewer) ───────────

// shutdown is called by Wails before the app exits.
func (a *App) shutdown(ctx context.Context) {
	log.Println("应用关闭中……")
	if a.chatCancel != nil {
		a.chatCancel()
	}
	if a.mcpServer != nil {
		if err := a.mcpServer.Stop(); err != nil {
			log.Printf("⚠ EverEvo MCP 服务停止失败: %v", err)
		} else {
			log.Println("EverEvo MCP 服务已停止")
		}
	}
	if a.a2aManager != nil {
		_ = a.a2aManager.StopServer()
		log.Println("A2A Agent 服务已停止")
	}
	if a.feishuClient != nil {
		a.feishuClient.Stop()
		log.Println("飞书机器人连接已关闭")
	}
	if a.mcpClient != nil {
		for _, s := range a.mcpClient.ListServers() {
			_ = a.mcpClient.Disconnect(s.ID)
		}
		log.Println("MCP 客户端连接已关闭")
	}
	if a.memoryStore != nil {
		_ = a.memoryStore.Close()
	}
	log.Println("应用已关闭")
}