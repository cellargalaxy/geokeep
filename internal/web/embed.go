// Package web 提供嵌入式静态资源。
// `//go:embed` 路径相对当前包，所以静态文件放在 internal/web/static/。
package web

import "embed"

//go:embed static
var FS embed.FS
