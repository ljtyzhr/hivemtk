package service

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// arch_gorm_guard_test.go —— E-4 新代码守卫（2026-10-24，规划 §4.4）。
//
// 规划口径：静态扫描禁止新增 `*gorm.DB` 字段或形参进入 service 层；
// 按域下沉是长期债，本轮不做，但基线写入门禁只许降。
//
// ============================================================================
// 2026-10-24 第二轮：度量改为 AST（只数代码），并把两类计数分开
//
// 旧版用 `grep -rnE '\*gorm\.DB'` 数**文本行**，两个后果都是反的激励：
//   - 注释也算数。实测 181 处里有 **21 处是注释**（多数还是"本 service 不持有
//     *gorm.DB"这类**已经修好**的说明）⇒ 债务在降、数字在涨；
//   - `func(tx *gorm.DB)` 事务回调与"握句柄"混在一个数里，而五层架构 §2 的依赖表
//     明确写着 Service 允许 `gorm（仅事务）`。
//
// 现在分两档，各自"只许降"：
//   - 句柄档（handle）：字段/形参里出现的 `*gorm.DB` —— §3.2 明令阻断的东西；
//   - 事务档（txCallback）：`func(tx *gorm.DB) error` 这种事务回调 —— 架构允许，
//     但**只许出现在 `X.Transaction(...)` 的实参位置**（见 TestTxCallbacksAreInTransactionPosition）。
//
// 两档都用 go/ast 解析（注释天然不在语法树里），不再 grep 文本：
// 数出来的东西必须等于"代码里真的有一个 *gorm.DB 类型"。
// ============================================================================

const (
	// gormGuardMaxHandles 句柄档基线（字段/形参）。**只许降**：下沉一域就把这个数改小。
	gormGuardMaxHandles = 9
	// gormGuardMaxTxCallbacks 事务回调档基线（架构允许，但只许降：能不下沉 tx 的就不用 tx）。
	gormGuardMaxTxCallbacks = 1
	gormGuardScope          = "internal/service/" // 含 internal/geo/service 等子包（cwd 相对）
)

// gormGuardScan 统计一个目录树里的两档出现次数与文件数。
func gormGuardScan(t *testing.T, root string) (handles, txCallbacks int, files int) {
	t.Helper()
	fset := token.NewFileSet()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		fileHas := false
		var visit func(n ast.Node, inCallback bool)
		visit = func(n ast.Node, inCallback bool) {
			ast.Inspect(n, func(m ast.Node) bool {
				switch v := m.(type) {
				case *ast.FuncDecl:
					// 声明自身形参 = 服务向外界要句柄（句柄档）；其体内再出现的
					// 函数字面量形参才是事务回调（事务档）。**返回类型也要数**：
					// `dbHandle() *gorm.DB` 就是把句柄递出去（本轮实测漏过 4 处）。
					h0, t0, f0 := classifyParams(v.Type, false)
					hr, fr := countResults(v.Type)
					handles += h0 + hr
					txCallbacks += t0
					fileHas = fileHas || f0 || fr
					if v.Body != nil {
						visit(v.Body, true)
					}
					return false
				case *ast.FuncLit:
					h1, t1, f1 := classifyParams(v.Type, true)
					hr1, fr1 := countResults(v.Type)
					handles += h1 + hr1
					txCallbacks += t1
					fileHas = fileHas || f1 || fr1
					visit(v.Body, true)
					return false
				case *ast.StructType:
					if v.Fields != nil {
						for _, fld := range v.Fields.List {
							if fld.Type != nil {
								h, tx, has := classifyExpr(fld.Type, false)
								handles += h
								txCallbacks += tx
								fileHas = fileHas || has
							}
						}
					}
					return false
				case *ast.StarExpr:
					if isGormDB(v.X) {
						fileHas = true
						if inCallback {
							txCallbacks++
						} else {
							handles++
						}
						return false
					}
				}
				return true
			})
		}
		visit(f, false)
		if fileHas {
			files++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("扫描 %s 失败: %v", root, err)
	}
	return handles, txCallbacks, files
}

func isGormDB(x ast.Expr) bool {
	sel, ok := x.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == "gorm" && sel.Sel.Name == "DB"
}

func TestArchServiceNoNewGormDB(t *testing.T) {
	handles, txCallbacks, files := gormGuardScan(t, ".")
	t.Logf("service 层 *gorm.DB：句柄档 %d 处（基线 %d）／事务回调档 %d 处（基线 %d）／涉及文件 %d",
		handles, gormGuardMaxHandles, txCallbacks, gormGuardMaxTxCallbacks, files)

	if handles > gormGuardMaxHandles {
		t.Fatalf("service 层新增 *gorm.DB 句柄（%d > 基线 %d）：按五层架构 §3.2，Service 直访 db 被阻断，新增应走 Repository 注入",
			handles, gormGuardMaxHandles)
	}
	if handles < gormGuardMaxHandles {
		t.Fatalf("句柄档基线收窄（现 %d < 基线 %d）：这是好事，但请把 gormGuardMaxHandles 下调到实际值——棘轮基线不许高于现实",
			handles, gormGuardMaxHandles)
	}
	if txCallbacks > gormGuardMaxTxCallbacks {
		t.Fatalf("service 层事务回调新增（%d > 基线 %d）：事务回调是架构允许的**例外**，例外只许变小；新代码请优先用 Repository 的 WithTx 接缝",
			txCallbacks, gormGuardMaxTxCallbacks)
	}
	if txCallbacks < gormGuardMaxTxCallbacks {
		t.Fatalf("事务回调档收窄（现 %d < 基线 %d）：请把 gormGuardMaxTxCallbacks 下调到实际值", txCallbacks, gormGuardMaxTxCallbacks)
	}
}

// TestGormGuardMetricHasTeeth 度量本身的自检：三种形态必须各自落对档。
//
// 立项理由：这一轮改的就是"度量把注释算成代码"——如果自检缺位，下一次有人再把它改回
// grep 文本、或把事务回调并回句柄档，所有读数都会静默失真，而门照样绿。
func TestGormGuardMetricHasTeeth(t *testing.T) {
	dir := t.TempDir()
	src := `package service

import "gorm.io/gorm"

// 注释里的 *gorm.DB 不算数（这正是本轮修掉的度量缺陷）
type Svc struct {
	db *gorm.DB        // 句柄档：字段
}

func NewSvc(db *gorm.DB) *Svc { return &Svc{db: db} } // 句柄档：形参

func (s *Svc) Run() error {
	return s.db.Transaction(func(tx *gorm.DB) error { // 事务档：回调形参
		return nil
	})
}
`
	if err := os.WriteFile(filepath.Join(dir, "fixture.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("写夹具失败: %v", err)
	}
	handles, txCallbacks, files := gormGuardScan(t, dir)
	if handles != 2 {
		t.Errorf("句柄档 = %d，期望 2（结构体字段 + 构造器形参；注释里的那处**不许**计入）", handles)
	}
	if txCallbacks != 1 {
		t.Errorf("事务档 = %d，期望 1（Transaction 回调的形参）", txCallbacks)
	}
	if files != 1 {
		t.Errorf("涉及文件 = %d，期望 1", files)
	}
}

// countParams 数一个函数类型的形参里出现了几处 *gorm.DB。
// inCallback=true 表示这是一层"回调函数类型"（func(tx *gorm.DB) error），
// 其形参记事务档；false 表示声明自身形参，记句柄档。
func countParams(ft *ast.FuncType, inCallback bool) int {
	h, tx, _ := classifyParams(ft, inCallback)
	_ = tx
	return h + tx
}

func classifyParams(ft *ast.FuncType, inCallback bool) (handles, txCallbacks int, has bool) {
	if ft == nil || ft.Params == nil {
		return 0, 0, false
	}
	for _, p := range ft.Params.List {
		if p.Type == nil {
			continue
		}
		h, tx, f := classifyExpr(p.Type, inCallback)
		handles += h
		txCallbacks += tx
		has = has || f
	}
	return handles, txCallbacks, has
}

// classifyExpr 把一处类型表达式归到两档。递归一层是为了
// `func(tx *gorm.DB) error` 这种"回调类型作为形参"的形态：
// 外层是形参（句柄档的载体），里面那个 *gorm.DB 才是事务档。
func classifyExpr(e ast.Expr, inCallback bool) (handles, txCallbacks int, has bool) {
	switch v := e.(type) {
	case *ast.StarExpr:
		if isGormDB(v.X) {
			if inCallback {
				return 0, 1, true
			}
			return 1, 0, true
		}
	case *ast.FuncType:
		return classifyParams(v, true)
	case *ast.ArrayType:
		return classifyExpr(v.Elt, inCallback)
	case *ast.Ellipsis:
		return classifyExpr(v.Elt, inCallback)
	case *ast.MapType:
		h1, t1, f1 := classifyExpr(v.Key, inCallback)
		h2, t2, f2 := classifyExpr(v.Value, inCallback)
		return h1 + h2, t1 + t2, f1 || f2
	}
	return 0, 0, false
}

func hasGormInParams(ft *ast.FuncType) bool {
	_, _, has := classifyParams(ft, false)
	return has
}

// countResults 数返回类型里的 *gorm.DB：把句柄交回调用方同样是"service 握句柄"。
func countResults(ft *ast.FuncType) (int, bool) {
	if ft == nil || ft.Results == nil {
		return 0, false
	}
	n := 0
	has := false
	for _, r := range ft.Results.List {
		if r.Type == nil {
			continue
		}
		h, _, f := classifyExpr(r.Type, false)
		n += h
		has = has || f
	}
	return n, has
}
