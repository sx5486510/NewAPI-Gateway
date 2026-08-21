package model

// 客户端类型常量，与 middleware/agg_token_auth.go:identifyClientType 的返回值一致。
const (
	ClientTypeCodex   = "codex"
	ClientTypeCC      = "cc"
	ClientTypeGeneric = ""
)

// EvaluateClientRestriction 是客户端准入的唯一判定入口。ModelRoute 与
// ProviderToken 的 IsClientAllowed 都委托给它，保证两层串行与运算时语义一致。
//
// 语义：
//   - 三个开关都为 false：不限制，放行所有客户端
//   - allowCodex/allowCC 任一为 true：线路"保留给"指定客户端，其余（含泛客户端）拒绝
//   - blockClients 为 true：线路"禁止"已识别客户端，只放行泛客户端
//
// blockClients 与 allow* 冲突时 blockClients 优先，与 NormalizeClientRestrictions 一致。
func EvaluateClientRestriction(clientType string, allowCodex, allowCC, blockClients bool) bool {
	if blockClients {
		allowCodex = false
		allowCC = false
	}

	if !allowCodex && !allowCC && !blockClients {
		return true
	}

	if blockClients {
		return clientType == ClientTypeGeneric
	}

	switch clientType {
	case ClientTypeCodex:
		return allowCodex
	case ClientTypeCC:
		return allowCC
	default:
		// 泛客户端与未来新增的未知客户端类型：受限线路一律拒绝。
		return false
	}
}
