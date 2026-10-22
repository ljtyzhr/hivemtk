package dto

// 卡片「是否激活」的三态处理（缺省 / 显式 true / 显式 false）。
//
// 坑：非指针 bool + gorm `default:true` 是一对会互相吃掉的组合。
// GORM 认为「零值 = 调用方没赋值」，于是把 false 从 INSERT 里剔除，交给列默认值兜底，
// 而列默认值是 true —— 于是「显式建一张禁用的卡片」会落库成激活，响应体也回 true。
// 用户在弹窗里把「激活」开关关掉再保存，界面却显示它还活着。
//
// 判据（三条同时成立才算 bug，不能只测其中一条）：
//  1. 模型字段是**非指针** bool 且带 `default:true`（指针不受影响：指针的 nil 才是零值）；
//  2. 仓储走**结构体形态**的 Create（map 形态按值落列，false 就是 false）；
//  3. 请求 DTO 用非指针 bool 接收（那就没法区分"没传"和"传了 false"）。
//
// 修法分两步，缺一不可：
//  1. 模型层去掉 `default:true` —— 只负责把 service 算好的值原样落库；
//  2. DTO 层改成 *bool + service 层用本文件的口径取值 —— 唯一知道"用户到底传没传"的地方。
//
// 列上的 DEFAULT true 保留给裸 SQL 写入（迁移脚本、运维脚本），不影响本条路径。

// BoolOrDefault 把「可空布尔」落成实际布尔值：
// nil（请求里没这个字段）⇒ def；显式 true/false ⇒ 原样透传。
//
// 用在卡片 is_active 这类「缺省即开」的字段上：把缺省语义留在 service 层，
// 模型层不再有资格替调用方猜值。
func BoolOrDefault(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

// BoolPtrOrDefaultTrue 是「缺省即开」的三态取值：nil ⇒ true。
func BoolPtrOrDefaultTrue(v *bool) bool {
	return BoolOrDefault(v, true)
}
