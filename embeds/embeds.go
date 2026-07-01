package embeds

import (
	_ "embed"
)

// DefaultConfigYAML 是内嵌的默认配置文件内容（带完整注释）
//
//go:embed config.yaml
var DefaultConfigYAML []byte
