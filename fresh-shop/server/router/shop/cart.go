package shop

import (
	"fresh-shop/server/api/v1"
	"fresh-shop/server/middleware"
	"github.com/gin-gonic/gin"
)

type CartRouter struct {
}

// InitCartRouter 初始化 Cart 路由信息
func (s *CartRouter) InitCartRouter(Router *gin.RouterGroup) {
	cartRouter := Router.Group("cart").Use(middleware.OperationRecord())
	var cartApi = v1.ApiGroupApp.ShopApiGroup.CartApi
	{
		cartRouter.POST("createCart", cartApi.CreateCart)             // 添加购物车 Cart
		cartRouter.DELETE("deleteCart", cartApi.DeleteCart)           // 删除Cart
		cartRouter.DELETE("deleteCartByIds", cartApi.DeleteCartByIds) // 批量删除Cart
		cartRouter.PUT("updateCart", cartApi.UpdateCart)              // 更新Cart
		cartRouter.GET("getCartList", cartApi.GetCartList)            // 获取Cart列表
	}
	{
	}
}