package shop

import (
	"fresh-shop/server/api/v1"
	"fresh-shop/server/middleware"
	"github.com/gin-gonic/gin"
)

type FavoritesRouter struct {
}

// InitFavoritesRouter 初始化 Favorites 路由信息
func (s *FavoritesRouter) InitFavoritesRouter(Router *gin.RouterGroup) {
	favoritesRouter := Router.Group("favorites").Use(middleware.OperationRecord())
	favoritesRouterWithoutRecord := Router.Group("favorites")
	var favoritesApi = v1.ApiGroupApp.ShopApiGroup.FavoritesApi
	{
		favoritesRouter.DELETE("deleteFavorites", favoritesApi.DeleteFavorites)           // 删除Favorites
		favoritesRouter.DELETE("deleteFavoritesByIds", favoritesApi.DeleteFavoritesByIds) // 批量删除Favorites
		favoritesRouter.POST("favorites", favoritesApi.Favorites)                         // 收藏 Favorites
	}
	{
		favoritesRouterWithoutRecord.GET("findFavorites", favoritesApi.FindFavorites)       // 根据ID获取Favorites
		favoritesRouterWithoutRecord.GET("getFavoritesList", favoritesApi.GetFavoritesList) // 获取用户Favorites列表
	}
}
