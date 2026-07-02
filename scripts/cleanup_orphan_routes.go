package main

import (
	"fmt"
	"log"

	"NewAPI-Gateway/model"
)

// 清理孤儿路由：即 provider_id 指向已删除供应商的 model_routes 记录
func main() {
	// 初始化数据库
	if err := model.InitDB(); err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}

	// 查找孤儿路由
	var orphanRoutes []model.ModelRoute
	err := model.DB.Table("model_routes AS mr").
		Select("mr.*").
		Joins("LEFT JOIN providers AS p ON p.id = mr.provider_id").
		Where("p.id IS NULL").
		Find(&orphanRoutes).Error

	if err != nil {
		log.Fatalf("查询孤儿路由失败: %v", err)
	}

	if len(orphanRoutes) == 0 {
		fmt.Println("✅ 未发现孤儿路由，数据健康！")
		return
	}

	// 打印孤儿路由详情
	fmt.Printf("⚠️  发现 %d 条孤儿路由（供应商已删除）：\n\n", len(orphanRoutes))
	for _, route := range orphanRoutes {
		fmt.Printf("  - 路由 ID: %d | 模型: %s | 供应商 ID: %d (已删除) | Token ID: %d\n",
			route.Id, route.ModelName, route.ProviderId, route.ProviderTokenId)
	}

	// 询问确认
	fmt.Print("\n是否删除这些孤儿路由？[y/N]: ")
	var confirm string
	fmt.Scanln(&confirm)

	if confirm != "y" && confirm != "Y" {
		fmt.Println("❌ 已取消清理操作")
		return
	}

	// 执行批量删除
	routeIds := make([]int, len(orphanRoutes))
	for i, route := range orphanRoutes {
		routeIds[i] = route.Id
	}

	result := model.DB.Where("id IN ?", routeIds).Delete(&model.ModelRoute{})
	if result.Error != nil {
		log.Fatalf("删除孤儿路由失败: %v", result.Error)
	}

	fmt.Printf("\n✅ 成功清理 %d 条孤儿路由！\n", result.RowsAffected)
}
