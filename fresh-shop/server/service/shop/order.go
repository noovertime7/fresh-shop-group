package shop

import (
	"errors"
	"fmt"
	"fresh-shop/server/global"
	"fresh-shop/server/model/common/request"
	"fresh-shop/server/model/shop"
	shopReq "fresh-shop/server/model/shop/request"
	shopResp "fresh-shop/server/model/shop/response"
	sysModel "fresh-shop/server/model/system"
	systemReq "fresh-shop/server/model/system/request"
	"fresh-shop/server/model/wechat/response"
	"fresh-shop/server/service/common"
	"fresh-shop/server/service/wechat"
	"fresh-shop/server/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"strconv"
	"strings"
	"time"
)

type OrderService struct {
}

// CreateOrder 创建Order记录
// Author [likfees](https://github.com/likfees)
func (orderService *OrderService) CreateOrder(order shop.Order, userClaims *systemReq.CustomClaims, clientIP string) (resp *response.CreateOrderResp, err error) {
	var user sysModel.SysUser
	if err := global.DB.Where("id = ?", order.UserId).First(&user).Error; err != nil {
		global.SugarLog.Errorf("创建订单时查询用户信息异常, err:%v \n", err)
		return nil, errors.New("用户查询失败")
	}

	// 获取收货地址信息
	address := shop.UserAddress{}
	addressName := ""
	if order.AddressId > 0 {
		if errors.Is(global.DB.Where("id = ?", order.AddressId).First(&address).Error, gorm.ErrRecordNotFound) {
			return nil, errors.New("收货地址不存在")
		}
		if *address.Sex == 1 {
			addressName = addressName + "先生"
		} else {
			addressName = addressName + "女士"
		}
	}
	var cartList []shop.Cart
	var orderDetailList []shop.OrderDetails

	if order.PointGoodsId != 0 { // 积分商品
		var goodsInfo shop.Goods
		if err := global.DB.Where("id = ? and goods_area = 1", order.PointGoodsId).Preload("Images").First(&goodsInfo).Error; err != nil {
			global.SugarLog.Errorf("创建订单时查询积分商品信息异常, err:%v \n", err)
			return nil, errors.New("商品查询失败")
		}
		c := shop.Cart{
			GoodsId:    utils.Pointer(order.PointGoodsId),
			UserId:     order.UserId,
			SpecType:   0,
			SpecItemId: 0,
			Num:        1,
			Checked:    utils.Pointer(1),
			Goods:      goodsInfo,
		}
		cartList = append(cartList, c)

	} else { // 普通商品
		// 获取购物车已选中的商品数据
		global.DB.Where("user_id = ? and checked = 1", order.UserId).Preload("Goods.Images").Find(&cartList)
		if len(cartList) <= 0 {
			global.SugarLog.Errorf("创建订单时查询商品信息异常, err:%v \n", err)
			return nil, errors.New("商品查询失败")
		}
	}
	pointCfg, err := common.GetSysConfig("point")
	pointSwitch := true
	if err != nil {
		if errors.Is(err, common.ErrConfigDisabled) {
			pointSwitch = false
		} else {
			global.SugarLog.Errorf("创建订单时查询积分配置参数异常, err:%v \n", err)
			return
		}
	}
	// 获取取餐号码
	if *order.ShipmentType == 1 {
		// 获取到最后一条有取餐号码的订单
		var pickOrder shop.Order
		if pickErr := global.DB.Select("pick_up_number").Where("pick_up_number is not null and pick_up_number <> 0").Order("id desc").First(&pickOrder).Error; pickErr == nil {
			order.PickUpNumber = pickOrder.PickUpNumber + 1
		} else {
			order.PickUpNumber = 101
		}
	}

	// 判断库存是否充足  以后可以上锁，解决高并发
	for _, c := range cartList {
		// 购物车数量大于库存
		if c.Num > *c.Goods.Store {
			global.SugarLog.Errorf("创建订单使库存不足 goodsId:%d, 购买数量:%d, 库存数量:%d \n", c.Goods.ID, c.Num, *c.Goods.Store)
			return nil, errors.New("商品库存不足")
		}
		// 计算总数量
		order.Num = order.Num + c.Num
		if order.PointGoodsId != 0 { // 积分商品
			order.Total = *c.Goods.CostPrice
		} else {
			// 计算总金额 如果优惠价小于成本价
			if *c.Goods.Price > 0 && *c.Goods.Price < *c.Goods.CostPrice {
				order.Total += float64(c.Num) * *c.Goods.Price
			} else {
				order.Total += float64(c.Num) * *c.Goods.CostPrice
			}
		}

		// 组织订单详情数据
		imgUrl := ""
		if len(c.Goods.Images) > 0 {
			imgUrl = c.Goods.Images[0].Url
		}
		orderDetail := shop.OrderDetails{}
		orderDetail.GoodsId = c.Goods.ID
		orderDetail.GoodsName = c.Goods.Name
		orderDetail.GoodsImage = imgUrl
		orderDetail.Unit = c.Goods.Unit
		orderDetail.Num = c.Num
		orderDetail.Price = *c.Goods.Price
		orderDetail.Total = 0
		if order.PointGoodsId != 0 { // 积分商品
			orderDetail.Total = *c.Goods.CostPrice
		} else {
			// 计算单个商品多个数量的总金额
			if *c.Goods.Price > 0 && *c.Goods.Price < *c.Goods.CostPrice {
				orderDetail.Total = float64(c.Num) * *c.Goods.Price
			} else {
				orderDetail.Total = float64(c.Num) * *c.Goods.CostPrice
			}
		}

		// 规格id 现在只开发了单规格订单，多规格以后在支持
		orderDetail.SpecId = 0
		spec := ""
		if *c.Goods.Weight > 0 {
			spec = fmt.Sprintf("%dg", *c.Goods.Weight)
		}
		if strings.TrimSpace(spec) == "" {
			spec = c.Goods.Unit
		} else {
			spec = spec + "/" + c.Goods.Unit
		}
		orderDetail.SpecKeyName = spec
		// 计算赠送积分
		if pointSwitch && order.PointGoodsId == 0 {
			point, err := strconv.Atoi(pointCfg)
			if err != nil {
				global.SugarLog.Errorf("创建订单详情信息时转换积分配置参数异常, err:%v \n", err)
				return nil, err
			}
			orderDetail.GiftPoints = orderDetail.Total * (float64(point) / 100)
		}
		orderDetailList = append(orderDetailList, orderDetail)
	}

	// 设置订单基本信息
	order.OrderSn = utils.GenerateOrderNumber("SN")
	if order.PointGoodsId > 0 { // 积分商品
		order.GoodsArea = utils.Pointer(1)
		order.Payment = utils.Pointer(4) // 积分支付
		order.Status = utils.Pointer(1)  // 已付款状态
		order.Finish = order.Total
		order.PayTime = utils.Pointer(time.Now())
	} else { // 普通商品
		order.GoodsArea = utils.Pointer(0)
		order.Payment = utils.Pointer(2) // 默认是微信支付
		order.Status = utils.Pointer(0)  // 未付款状态
	}
	order.ShipmentName = addressName
	order.ShipmentMobile = address.Mobile
	order.ShipmentAddress = address.Address + address.Title + address.Detail
	order.StatusCancel = utils.Pointer(0)
	order.StatusRefund = utils.Pointer(0)
	// 计算总赠送积分
	if pointSwitch && order.PointGoodsId == 0 {
		point, err := strconv.Atoi(pointCfg)
		if err != nil {
			global.SugarLog.Errorf("创建订单时转换积分配置参数异常, err:%v \n", err)
			return nil, err
		}
		// 公式 总金额 * n%
		order.GiftPoints = order.Total * (float64(point) / 100)
	}

	log := fmt.Sprintf("[OrderService] CreateOrder submit data:%+v; \n", order)
	// 启动事务
	txDB := global.DB.Begin()
	// 创建订单
	if err = global.DB.Create(&order).Error; err != nil {
		txDB.Rollback()
		global.SugarLog.Errorf("log:%s,err:%v \n", log, err)
		return nil, errors.New("订单创建失败")
	}
	if order.ID == 0 {
		txDB.Rollback()
		global.SugarLog.Errorf("log:%s, err: 创建订单后订单ID获取失败 \n", log)
		return nil, errors.New("订单创建失败")
	}
	// 创建订单详情
	// 设置订单详情 orderId
	for k, _ := range orderDetailList {
		orderDetailList[k].OrderId = order.ID
	}
	if err = global.DB.Create(&orderDetailList).Error; err != nil {
		txDB.Rollback()
		global.SugarLog.Errorf("log:%s,err:%v \n", log, err)
		return nil, errors.New("订单详情创建失败")
	}
	// 扣减库存
	for _, v := range cartList {
		if err = global.DB.Model(&shop.Goods{}).Where("id = ?", v.GoodsId).Update("store", gorm.Expr("store - ?", v.Num)).Error; err != nil {
			txDB.Rollback()
			global.SugarLog.Errorf("log:%s,err:%v \n", log, err)
			return nil, errors.New("库存扣减失败")
		}
	}
	if order.PointGoodsId == 0 {
		// 删除购物车列表
		if err = global.DB.Delete(&cartList).Error; err != nil {
			txDB.Rollback()
			global.SugarLog.Errorf("log:%s,err:%v \n", log, err)
			return nil, errors.New("购物车删除失败")
		}
	}

	if order.PointGoodsId > 0 {
		// 扣减积分
		f := common.NewFinance(common.OptionTypeCASH, 1, user.ID, user.Username, -order.Total, order.OrderSn, user.ID, user.Username, "购买积分商品")
		err := common.AccountUnifyDeduction(common.POINT, f)
		if err != nil {
			txDB.Rollback()
			global.SugarLog.Errorf("log:%s, 积分扣减失败 err:%v \n", log, err)
			return nil, err
		}
	}

	// 提交事务
	txDB.Commit()
	//jsApiData := &orderPay.Config{}
	//if order.PointGoodsId == 0 {
	//	// 发起 JSAIP 支付返回参数
	//	err, jsApiData = wechat.JSAPIPay(userClaims.OpenId, order.OrderSn, order.ID, order.Total, clientIP)
	//	if err != nil {
	//		global.SugarLog.Errorf("log:%s, 微信 JsApi 发起调用异常, err: %v \n", log, err)
	//		return
	//	}
	//}
	resp = &response.CreateOrderResp{
		Order: order,
		//Pay:   *jsApiData,
	}
	return
}

// OrderPay 支付 Order, 返回微信支付所需要的参数
// Author [likfees](https://github.com/likfees)
func (orderService *OrderService) OrderPay(order shop.Order, userClaims *systemReq.CustomClaims, clientIP string) (resp *response.CreateOrderResp, err error) {
	log := fmt.Sprintf("[OrderService] OrderPay orderId:%d; \n", order.ID)
	// 查询订单信息
	if errors.Is(global.DB.Where("id = ?", order.ID).First(&order).Error, gorm.ErrRecordNotFound) {
		global.SugarLog.Errorf("log:%s,err:订单不存在 \n", log)
		return nil, errors.New("订单不存在")
	}
	if *order.Status == 1 {
		global.SugarLog.Errorf("log:%s,err:订单已支付 \n", log)
		return nil, errors.New("订单已支付")
	} else if *order.Status != 0 {
		global.SugarLog.Errorf("log:%s,err:订单状态不正确 \n", log)
		return nil, errors.New("订单状态不正确")
	}
	// 发起 JSAIP 支付返回参数
	err, jsApiData := wechat.JSAPIPay(userClaims.OpenId, order.OrderSn, order.ID, order.Total, clientIP)
	if err != nil {
		global.SugarLog.Errorf("log:%s, 微信 JsApi 发起调用异常, err: %v \n", log, err)
		return
	}
	resp = &response.CreateOrderResp{
		Order: order,
		Pay:   *jsApiData,
	}
	return
}

// OrderDeliver 订单发货
func (orderService *OrderService) OrderDeliver(order shop.Order) (err error) {
	return
}

// DeleteOrder 删除Order记录
// Author [likfees](https://github.com/likfees)
func (orderService *OrderService) DeleteOrder(order shop.Order) (err error) {
	var detail shop.OrderDetails
	err = global.DB.Transaction(func(tx *gorm.DB) error {
		err = tx.Where("order_id = ?", order.ID).Delete(&detail).Error
		if err != nil {
			global.SugarLog.Errorf("删除订单详情失败 %d, err: %v", order.ID, err)
			return err
		}
		err = tx.Delete(&order).Error
		if err != nil {
			global.SugarLog.Errorf("删除订单失败 %d, err: %v", order.ID, err)
			return err
		}
		return nil
	})
	return err
}

// CancelOrder 取消订单
// Author [likfees](https://github.com/likfees)
func (orderService *OrderService) CancelOrder(order shop.Order) (err error) {
	cancelType := 1 // 默认用户取消
	if order.StatusCancel != nil && *order.StatusCancel > 1 {
		cancelType = *order.StatusCancel
	}
	if errors.Is(global.DB.Where("id = ?", order.ID).First(&order).Error, gorm.ErrRecordNotFound) {
		return errors.New("订单不存在")
	}
	// 发货 收货状态不允许取消
	if *order.Status >= 2 {
		return errors.New("订单不允许取消")
	}
	// 如果订单已支付需要进行退款
	order.StatusCancel = &cancelType
	order.CancelTime = utils.Pointer(time.Now())
	err = global.DB.Where("id = ?", order.ID).Updates(&order).Error
	return err
}

// DeleteOrderByIds 批量删除Order记录
// Author [likfees](https://github.com/likfees)
func (orderService *OrderService) DeleteOrderByIds(ids request.IdsReq) (err error) {
	err = global.DB.Delete(&[]shop.Order{}, "id in ?", ids.Ids).Error
	return err
}

// UpdateOrder 更新Order记录
// Author [likfees](https://github.com/likfees)
func (orderService *OrderService) UpdateOrder(order shop.Order) (err error) {
	err = global.DB.Save(&order).Error
	return err
}

// GetOrder 根据id获取Order记录
// Author [likfees](https://github.com/likfees)
func (orderService *OrderService) GetOrder(id uint) (order shop.Order, err error) {
	err = global.DB.Where("id = ?", id).
		Preload("OrderDetails.Goods").
		Preload("OrderReturn.Details").
		Preload("OrderDelivery.UserDelivery").
		First(&order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return order, errors.New("订单不存在")
	}
	return
}

// FindUserOrderStatus 获取用户订单中数量
// Author [likfees](https://github.com/likfees)
func (orderService *OrderService) FindUserOrderStatus(userId uint) (resp shopResp.OrderStatusCountResponse, err error) {
	err = global.DB.Debug().
		Table("shop_order").
		Select("COUNT(CASE WHEN status = 0 and status_cancel = 0 THEN 1 ELSE null END ) as unpaid,COUNT(CASE WHEN status = 1 and status_cancel = 0 and status_refund = 0 THEN 1 ELSE null END ) as delivered,COUNT(CASE WHEN status = 2 and status_cancel = 0 and status_refund = 0 THEN 1 ELSE null END ) as shipped,COUNT(CASE WHEN status = 3 and status_cancel = 0 and status_refund = 0 THEN 1 ELSE null END) as success").
		Where("user_id = ?", userId).
		Scan(&resp).Error
	return
}

// OrderStatus 获取订单状态
// Author [likfees](https://github.com/likfees)
func (orderService *OrderService) OrderStatus(id uint) (status gin.H, err error) {
	var o shop.Order
	if errors.Is(global.DB.Select("status").Where("id = ?", id).First(&o).Error, gorm.ErrRecordNotFound) {
		return gin.H{}, errors.New("订单不存在")
	}
	result := gin.H{
		"status": o.Status,
	}
	return result, nil
}

// GetOrderInfoList 分页获取Order记录
// Author [likfees](https://github.com/likfees)
func (orderService *OrderService) GetOrderInfoList(info shopReq.OrderSearch) (list []shop.Order, total int64, err error) {
	limit := info.PageSize
	offset := info.PageSize * (info.Page - 1)
	// 创建db
	db := global.DB.Debug().Model(&shop.Order{}).Preload("OrderDetails").Preload("OrderDelivery").Joins("OrderReturn")
	var orders []shop.Order
	// 如果有条件搜索 下方会自动创建搜索语句

	if info.Status != nil {
		if *info.Status == 0 || *info.Status == 1 || *info.Status == 2 || *info.Status == 3 { // 未付款
			db = db.Where("shop_order.status = ? and shop_order.status_cancel = 0 and shop_order.status_refund = 0", info.Status)
		} else if *info.Status == 10 { // 售后订单
			db = db.Where("OrderReturn.order_id = shop_order.id and shop_order.status_cancel = 0")
		}
	}

	if info.UserId != nil {
		db = db.Where("shop_order.user_id = ?", info.UserId)
	}
	if info.GoodsArea != nil {
		db = db.Where("shop_order.goods_area = ?", info.GoodsArea)
	}
	if info.StartCreatedAt != nil && info.EndCreatedAt != nil {
		db = db.Where("shop_order.created_at BETWEEN ? AND ?", info.StartCreatedAt, info.EndCreatedAt)
	}
	if info.OrderSn != "" {
		db = db.Where("shop_order.order_sn LIKE ?", "%"+info.OrderSn+"%")
	}
	if info.ShipmentName != "" {
		db = db.Where("shop_order.shipment_name LIKE ?", "%"+info.ShipmentName+"%")
	}
	if info.ShipmentMobile != "" {
		db = db.Where("shop_order.shipment_mobile LIKE ?", "%"+info.ShipmentMobile+"%")
	}
	if info.ShipmentAddress != "" {
		db = db.Where("shop_order.shipment_address LIKE ?", "%"+info.ShipmentAddress+"%")
	}
	if info.Payment != nil {
		db = db.Where("shop_order.payment = ?", info.Payment)
	}
	if info.StartShipmentTime != nil && info.EndShipmentTime != nil {
		db = db.Where("shop_order.shipment_time BETWEEN ? AND ? ", info.StartShipmentTime, info.EndShipmentTime)
	}
	if info.StartReceiveTime != nil && info.EndReceiveTime != nil {
		db = db.Where("shop_order.receive_time BETWEEN ? AND ? ", info.StartReceiveTime, info.EndReceiveTime)
	}
	if info.StartCancelTime != nil && info.EndCancelTime != nil {
		db = db.Where("shop_order.cancel_time BETWEEN ? AND ? ", info.StartCancelTime, info.EndCancelTime)
	}
	err = db.Count(&total).Error
	if err != nil {
		return
	}

	db = db.Order("shop_order.created_at desc")

	err = db.Limit(limit).Offset(offset).Find(&orders).Error
	return orders, total, err
}
