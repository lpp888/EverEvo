package tools

func registerToolboxTools() {
	Register(&ToolDef{
		Name:        "toolbox_list_models",
		Description: "列出工具箱中已探测到的可用模型，按类型分类（句向量/图像分类等），包含名称、类型和本地路径",
		Category:    "toolbox",
		Parameters: &ToolParams{
			Type:       "object",
			Properties: map[string]ToolProp{},
		},
	})

	Register(&ToolDef{
		Name:        "toolbox_embed",
		Description: "用句向量（sentence-embedding）模型将多段文本编码为嵌入向量。可用于计算文本相似度或语义搜索",
		Category:    "toolbox",
		Parameters: &ToolParams{
			Type: "object",
			Properties: map[string]ToolProp{
				"modelDir": {Type: "string", Description: "句向量模型的本地目录路径"},
				"texts":    {Type: "array", Description: "要编码的文本列表", Items: &ToolProp{Type: "string"}},
			},
			Required: []string{"modelDir", "texts"},
		},
	})
}
