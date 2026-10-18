package main

import (
	"context"
	"fmt"

	"hivemtk-user/internal/config"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/platform"
	"hivemtk-user/internal/service"
)

func main() {
	if err := config.LoadPlatform("config/platform.yaml"); err != nil {
		fmt.Println("LoadPlatform:", err)
	}
	db.InitDB()
	platform.SetMerchantKeyForTest("MKE2E")

	svc := service.NewModuleCatalogService()
	res, err := svc.Sync(context.Background())
	if err != nil {
		fmt.Println("Sync 失败:", err)
		return
	}
	fmt.Printf("同步完成: active=%d source=%s degraded=%v msg=%s\n",
		res.ActiveCount, res.Source, res.Degraded, res.Message)
}
