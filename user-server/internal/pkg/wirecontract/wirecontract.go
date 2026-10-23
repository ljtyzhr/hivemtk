// Package wirecontract 把"平台端 ⇄ 租户端（user-server）线上传的那些结构"从**活体 struct**
// 反射成一份可对比的清单（SaaS 审计 A10）。
//
// # 它解决的失效形态
//
// 两个仓各自定义同一批结构的 Go 类型：`RegisterReq`（平台）/ `RegisterMerchantReq`（租户）、
// `HeartbeatRequest` / `ReportHeartbeatReq`、`CreateOrderInput` / `CreateOrderInput`、
// `ModuleSyncSnapshot` / `ModuleSnapshot`…… 它们靠**人对齐**，而 Go 编译器看不见另一侧。
// 于是"租户端改名了一个 json tag"这种改动**两边都能编译、两边用例都能绿**，线上表现是
// 平台收到一个它不认识的键 → 静默忽略 → 该字段永远是零值。这类 bug 不报错、不进日志，
// 只在几个月后有人问"为什么这个开关改了没反应"时才被翻出来。
//
// # 为什么不是"共一份代码"
//
// 两端是两个独立库、两条演进节奏，`ConfigParam` 这类**各自库内**的同名模型共代码并不划算
// （审计 P2-6 已明确只对**跨端传递**的结构设事实源）。所以事实源取**形状清单**而不是共享包：
// 平台侧 DTO 是 wire 的权威方（它定义服务端接受什么），它把形状吐成
// `platform-server/docs/contracts/cross-repo-wire.json`；本仓把**自己那一侧**的形状吐成
// `docs/contracts/cross-repo-wire.json`；`scripts/check-wire-contract.py` 按**方向性**规则
// 比对两者（租户的字段必须是平台的子集，且平台 `binding:"required"` 的字段租户必须发）。
// 子集方向而不是相等，是因为平台侧合法地比租户侧多（`owned`/`menu_keys`/`asset_ids`
// 等只服务平台自身）。
//
// # 为什么是反射而不是手写清单
//
// 手写清单与 struct 之间没有任何绑定，改了 struct 清单不动就是"文档说 X"——正是本审计
// 三条判据第一条要挡的。反射出来的清单不可能与代码不一致：它就是代码。
package wirecontract

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"time"
)

// Manifest 是一份仓内结构形状清单（生成物，进 git，不得手改）。
type Manifest struct {
	Note    string            `json:"_note"`
	Version int               `json:"version"`
	Structs map[string]Struct `json:"structs"`

	// renames Go 类型名 ⇒ 清单规范名。不序列化：它是生成参数，不是清单内容。
	renames map[string]string
}

// Struct 一个具名类型在线上暴露的字段集合。
type Struct struct {
	Fields []Field `json:"fields"`
}

// Field 一个字段的**线上形状**。
//
// 只记 wire 层信息，不记 Go 类型名：`time.Time` 与 `string` 在线上都是 RFC3339 字符串，
// 把它们当成不一致会造出永久红（`ModuleSyncSnapshot.SyncedAt` 正是这一对）。
type Field struct {
	JSON      string `json:"json"`
	Kind      string `json:"kind"`
	Nullable  bool   `json:"nullable,omitempty"`
	OmitEmpty bool   `json:"omitempty,omitempty"`
	// Required 只有平台侧会填：`binding:"required"` 意味着"缺了它直接 400"，
	// 所以它反过来约束租户端必须发这个键。
	Required bool `json:"required,omitempty"`
	// Elem array 的元素 / object 的具名类型（标量数组为空串）
	Elem string `json:"elem,omitempty"`
}

// Wire 类型口径（只区分到"线上解析得出什么"这一层）
const (
	KindString  = "string"
	KindInteger = "integer"
	KindNumber  = "number"
	KindBoolean = "boolean"
	KindArray   = "array"
	KindObject  = "object"
	KindAny     = "any" // json.RawMessage / interface{}：线上形状不限
)

var (
	timeType = reflect.TypeOf(time.Time{})
	rawType  = reflect.TypeOf(json.RawMessage(nil))
)

// Describe 从一批具名根类型生成清单。roots 的 key 就是清单里的 struct 名。
func Describe(roots map[string]any) Manifest {
	return DescribeWithRenames(roots, nil)
}

// DescribeWithRenames 同 Describe，但把反射出来的 Go 类型名按 renames 改写成**规范名**。
//
// # 为什么需要它
//
// 跨仓比对的前提是"同一个线上结构在两侧的清单里叫同一个名字"，否则比对就得靠一张
// "谁对谁"的映射表，而那张表漏一行就等于那条子树**静默不再被判据覆盖**。
// 偏偏两侧的 Go 类型名合法地不同：平台 `service.MerchantModuleView` / 租户
// `platform.ModuleCatalogRow` 说的是同一个线上对象。于是让租户侧在生成清单时就把
// 嵌套类型名改写成平台侧的名字（根类型本来就用平台名，见 manifest_test.go），
// 两份清单就落进**同一个命名空间**：配对关系退化成键相等，映射表不再存在，
// 也就不存在"表漏了一行"这种失效。
//
// 改写不了的（新出现且没登记的嵌套类型）会以自己的 Go 名留在清单里，于是对面查不到
// 那个结构 ⇒ 红，而不是被悄悄跳过。
func DescribeWithRenames(roots map[string]any, renames map[string]string) Manifest {
	m := Manifest{
		Note: "生成物，勿手改：由 internal/wirecontract 的测试从活体 struct 反射生成（SaaS 审计 A10）",
		// Version 清单格式版本（不是契约版本）。改字段口径/聚合方式时 +1。
		Version: 1,
		Structs: map[string]Struct{},
		renames: renames,
	}
	for name, v := range roots {
		m.walk(reflect.TypeOf(v), m.canonical(name))
	}
	return m
}

// canonical 把 Go 类型名改写成清单里的规范名（未登记则原样保留）。
func (m Manifest) canonical(name string) string {
	if to, ok := m.renames[name]; ok {
		return to
	}
	return name
}

func (m Manifest) walk(t reflect.Type, name string) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	if _, seen := m.Structs[name]; seen {
		return
	}
	// 先占位再填字段：自引用结构不会无限递归。
	m.Structs[name] = Struct{}
	fields := m.fieldsOf(t)
	sort.Slice(fields, func(i, j int) bool { return fields[i].JSON < fields[j].JSON })
	m.Structs[name] = Struct{Fields: fields}
}

// fieldsOf 展开一个 struct 的线上字段。
//
// 内嵌结构（`f.Anonymous`）在这里**就地摊平**：`encoding/json` 对内嵌 struct 就是这么做的，
// 若把它当成一个普通字段，清单会与线上形状不符 —— 而清单不符的代价是这条判据开始说谎。
func (m Manifest) fieldsOf(t reflect.Type) []Field {
	fields := make([]Field, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" { // 未导出：不可能出现在 wire 上
			continue
		}
		ft := f.Type
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if f.Anonymous && ft.Kind() == reflect.Struct && f.Tag.Get("json") == "" {
			fields = append(fields, m.fieldsOf(ft)...)
			continue
		}
		jsonName, omitEmpty, skip := parseJSONTag(f.Tag.Get("json"))
		if skip {
			continue
		}
		if jsonName == "" && f.Tag.Get("json") == "" {
			// 没写 json tag ⇒ encoding/json 用 Go 字段名。如实记 Go 名（比对时与对面的
			// json 名对不上正是我们要看见的：这条字段在 wire 上叫的是 Go 名）。
			jsonName = f.Name
		}
		fld := Field{JSON: jsonName, OmitEmpty: omitEmpty, Required: strings.Contains(f.Tag.Get("binding"), "required")}
		ft = f.Type
		for ft.Kind() == reflect.Ptr {
			fld.Nullable = true
			ft = ft.Elem()
		}
		fld.Kind, fld.Elem = kindOf(ft)
		fld.Elem = m.canonical(fld.Elem)
		switch {
		case fld.Kind == KindArray && fld.Elem != "":
			m.walk(ft.Elem(), fld.Elem)
		case fld.Kind == KindObject && fld.Elem != "":
			// 具名 struct：元素类型就是它自己（map 没有具名元素，所以这里不会是 map）。
			m.walk(ft, fld.Elem)
		}
		fields = append(fields, fld)
	}
	return fields
}

// kindOf 返回 (线上口径, 具名元素类型名)。
func kindOf(t reflect.Type) (string, string) {
	switch t {
	case timeType, rawType:
		// time.Time 线上是字符串；json.RawMessage 线上形状由调用方决定（不限）。
		if t == timeType {
			return KindString, ""
		}
		return KindAny, ""
	}
	switch t.Kind() {
	case reflect.String:
		return KindString, ""
	case reflect.Bool:
		return KindBoolean, ""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return KindInteger, ""
	case reflect.Float32, reflect.Float64:
		return KindNumber, ""
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 { // []byte
			return KindAny, ""
		}
		k, elem := kindOf(t.Elem())
		if elem == "" && k == KindObject {
			elem = t.Elem().Name()
		}
		return KindArray, elem
	case reflect.Map:
		return KindObject, ""
	case reflect.Struct:
		return KindObject, t.Name()
	case reflect.Interface:
		return KindAny, ""
	}
	return KindAny, ""
}

// parseJSONTag 解析 json tag：返回 (名字, 是否 omitempty, 是否整字段跳过)。
func parseJSONTag(tag string) (name string, omitEmpty, skip bool) {
	if tag == "-" {
		return "", false, true
	}
	parts := strings.Split(tag, ",")
	name = parts[0]
	for _, opt := range parts[1:] {
		if opt == "omitempty" {
			omitEmpty = true
		}
	}
	return name, omitEmpty, false
}
