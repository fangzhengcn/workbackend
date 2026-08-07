package model

import "strconv"

// formatUint 是 uint64 转字符串的内部小工具。
func formatUint(v uint64) string { return strconv.FormatUint(v, 10) }
