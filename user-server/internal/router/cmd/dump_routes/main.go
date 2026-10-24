//go:build ignore

// dump_routes.go —— E-2 注解补齐的第一步：把生产路由表 dump 成 JSON。
// （见同批 scripts/backfill_swagger_annotations.py。）
//
// 与 route_surface_consistency_test.go 的 liveRoutes 同口径：Setup(engine, testDB)。
// 输出 [{method, path, handler}]，handler 形如 "(*Type).Func"。
//
// 构建：go run -tags ignore ./internal/router/cmd/dump_routes（需要测试库）。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"hivemtk-user/internal/model"
	dbutil "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/router"

	"github.com/gin-gonic/gin"
)

// tbStub 让 NewTestDB 的 testing.TB 依赖可运行（Cleanup/Fatal 有实做）。
type tbStub struct {
	testing.TB
}

func (tbStub) Cleanup(f func()) { defer f() } // 进程退出即清理，无需真正登记
func (tbStub) Fatal(args ...any) {
	fmt.Fprintln(os.Stderr, args...)
	os.Exit(1)
}
func (tbStub) Fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}
func (tbStub) Helper() {}
func (tbStub) Logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
	fmt.Fprintln(os.Stderr)
}
func (tbStub) Skipf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "SKIP: "+format, args...)
	os.Exit(1)
}
func (tbStub) Setenv(k, v string) { os.Setenv(k, v) }

func main() {
	var t tbStub
	os.Setenv("USER_WEB_DIST", os.TempDir())
	database := testutil.NewTestDB(t, &model.Opportunity{}, &model.Quote{}, &model.QuoteLineItem{},
		&model.ApprovalRequest{}, &model.SalesEvent{}, &model.SystemConfigKV{})
	dbutil.SetTestDB(database)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	router.Setup(engine, database)
	type row struct {
		Method  string `json:"method"`
		Path    string `json:"path"`
		Handler string `json:"handler"`
	}
	out := make([]row, 0, 2048)
	for _, ri := range engine.Routes() {
		out = append(out, row{Method: ri.Method, Path: ri.Path, Handler: ri.Handler})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
