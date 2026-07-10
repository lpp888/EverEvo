package tools

func registerSystemTools() {
	Register(&ToolDef{
		Name:        "system_info",
		Description: "获取系统静态信息：CPU 型号/核心/特性、内存总量、GPU 列表及驱动版本、Windows 版本",
		Category:    "system",
		Parameters: &ToolParams{
			Type:       "object",
			Properties: map[string]ToolProp{},
		},
		Annotations: &ToolAnnotations{ReadOnlyHint: true},
	})

	Register(&ToolDef{
		Name:        "system_dynamic",
		Description: "获取系统实时性能指标：CPU 使用率、内存使用率/可用量、各 GPU 使用率/显存、磁盘读写速度",
		Category:    "system",
		Parameters: &ToolParams{
			Type:       "object",
			Properties: map[string]ToolProp{},
		},
		Annotations: &ToolAnnotations{ReadOnlyHint: true},
	})

	Register(&ToolDef{
		Name:        "system_backends",
		Description: "检测所有推理后端的可用状态：ONNX Runtime 和 llama.cpp 是否可用及其 DLL 路径",
		Category:    "system",
		Parameters: &ToolParams{
			Type:       "object",
			Properties: map[string]ToolProp{},
		},
	})

	Register(&ToolDef{
		Name:        "system_config",
		Description: "获取当前用户配置：模型目录、默认后端、主题、语言、LLM API 配置等",
		Category:    "system",
		Parameters: &ToolParams{
			Type:       "object",
			Properties: map[string]ToolProp{},
		},
	})

	Register(&ToolDef{
		Name:        "proxy_get",
		Description: "获取当前 HTTP 代理配置状态：是否启用、代理地址",
		Category:    "system",
		Parameters:  &ToolParams{Type: "object", Properties: map[string]ToolProp{}},
	})
	Register(&ToolDef{
		Name:        "proxy_set",
		Description: "设置 HTTP 代理地址。格式如 http://127.0.0.1:7890",
		Category:    "system",
		Parameters: &ToolParams{
			Type: "object",
			Properties: map[string]ToolProp{
				"url": {Type: "string", Description: "代理 URL"},
			},
		},
	})
	Register(&ToolDef{
		Name:        "proxy_test",
		Description: "测试代理是否可用。发送 HTTP 请求验证连通性",
		Category:    "system",
		Parameters: &ToolParams{
			Type: "object",
			Properties: map[string]ToolProp{
				"url": {Type: "string", Description: "要测试的代理 URL"},
			},
		},
	})
	Register(&ToolDef{
		Name:        "proxy_toggle",
		Description: "启用或禁用 HTTP 代理",
		Category:    "system",
		Parameters: &ToolParams{
			Type: "object",
			Properties: map[string]ToolProp{
				"enabled": {Type: "boolean", Description: "true 启用 / false 禁用"},
			},
		},
	})
	Register(&ToolDef{
		Name:        "download_engine",
		Description: "下载推理后端引擎文件（如 ONNX Runtime DLL、llama.cpp 可执行文件）",
		Category:    "system",
		Parameters: &ToolParams{
			Type: "object",
			Properties: map[string]ToolProp{
				"key":     {Type: "string", Description: "引擎标识: onnxruntime / llamacpp"},
				"mirror":  {Type: "string", Description: "下载镜像源"},
				"variant": {Type: "string", Description: "变体: cpu / cuda / vulkan"},
			},
		},
	})

	// shell_exec — run an OS command with safety guards.
	Register(&ToolDef{
		Name:        "shell_exec",
		Description: "在本地 Shell 中执行命令并返回输出。可用于文件搜索、git 操作、npm/pip 安装、进程管理、系统配置等。命令在用户权限下运行，有 30s 默认超时和危险命令拦截。",
		Category:    "system",
		Parameters: &ToolParams{
			Type: "object",
			Properties: map[string]ToolProp{
				"command": {Type: "string", Description: "要执行的 shell 命令。Windows 使用 cmd /c 执行，支持管道和重定向。"},
				"cwd":     {Type: "string", Description: "工作目录。默认: 应用所在目录。"},
				"timeout": {Type: "integer", Description: "超时秒数，默认 30，最大 300。"},
			},
			Required: []string{"command"},
		},
	})

	// web_search — search the web via DuckDuckGo (free, no API key).
	Register(&ToolDef{
		Name:        "web_search",
		Description: "搜索互联网，返回标题、摘要和链接。基于 DuckDuckGo，免费无需 API Key。每次搜索返回前 8 条结果。",
		Category:    "system",
		Annotations: &ToolAnnotations{ReadOnlyHint: true},
		Parameters: &ToolParams{
			Type: "object",
			Properties: map[string]ToolProp{
				"query": {Type: "string", Description: "搜索关键词"},
			},
			Required: []string{"query"},
		},
	})
}
