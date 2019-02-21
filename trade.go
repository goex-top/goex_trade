package trade

import (
	"github.com/sumorf/bitmexwrap"
	"time"
)

var FORCE_ENTER = true
var TAKE = false
var SLIP = 1.0
var price_n = 1
var num_n = 0

var orderCheckInterval = time.Duration(1000)

func (em *EmaMacd) takeWithBestPrice(amount float64, depths []bitmexwrap.DepthInfo) (price float64) {
	total := 0.0
	for _, v := range depths {
		total += v.Amount
		if total >= amount {
			return v.Price
		}
	}
	return 0
}

func (em *EmaMacd) BuyLong(amount float64) (bool, *bitmexwrap.Order) {
	var putOrderRetry = 60 * (em.period / 5)
	var totalOrderRetry = 150 * (em.period / 5)

	var ord *bitmexwrap.Order
	var err error
	var state = 0
	var count = 0
	amount = utils.Float64Round(amount, num_n)
	for ; count < putOrderRetry; count++ {
		depth := utils.RE(em.exchange.GetDepth, 5).(bitmexwrap.Orderbook)
		var price float64
		if TAKE {
			price = utils.Float64Round(depth.Sells[0].Price+SLIP, price_n)
		} else {
			price = utils.Float64Round(depth.Buys[0].Price, price_n)
		}
		ord, err = em.exchange.OpenLong(price, amount, false, "")

		if err != nil {
			em.log.Warningln("BuyLong 失败，正在retry")
			time.Sleep(orderCheckInterval * time.Millisecond)
			continue
		} else {
			state = 1
			em.log.Warningln("BuyLong 挂单成功", price, amount)
			break
		}
	}
	if state == 0 {
		return false, nil
	}
	for ; count < totalOrderRetry; count++ {
		ord = utils.RE(em.exchange.GetOrder, ord.OrderID).(*bitmexwrap.Order)
		if ord.Status != "Filled" {
			time.Sleep(orderCheckInterval * time.Millisecond)
			continue
		} else {
			em.log.Warningln("BuyLong 挂单已成交")
			return true, ord
		}
	}
	if state == 1 {
		if FORCE_ENTER {
			remainAmount := ord.Amount - ord.DealAmount
			em.log.Warningln("remainAmount:", remainAmount)
			if remainAmount > 0 { //处理头寸
				time.Sleep(orderCheckInterval * time.Millisecond)
				depth := utils.RE(em.exchange.GetDepth, 20).(bitmexwrap.Orderbook)
				var price float64
				price = em.takeWithBestPrice(remainAmount, depth.Sells) + SLIP
				ord, _ = em.exchange.OrderAmend(ord.OrderID, price)
				em.log.Warningln("BuyLong 挂单未成交, 增加滑点后成交")
			}
		} else {
			utils.RE(em.exchange.CancelOrder, ord.OrderID)
			em.log.Warningln("BuyLong 挂单未成交， 取消后退出")
		}

		return true, ord
	}
	return true, ord
}

func (em *EmaMacd) SellShort(amount float64) (bool, *bitmexwrap.Order) {
	var putOrderRetry = 60 * (em.period / 5)
	var totalOrderRetry = 150 * (em.period / 5)

	var ord *bitmexwrap.Order
	var err error
	var state = 0
	var count = 0
	amount = utils.Float64Round(amount, num_n)
	for ; count < putOrderRetry; count++ {
		depth := utils.RE(em.exchange.GetDepth, 5).(bitmexwrap.Orderbook)
		var price float64
		if TAKE {
			price = utils.Float64Round(depth.Buys[0].Price-SLIP, price_n)
		} else {
			price = utils.Float64Round(depth.Sells[0].Price, price_n)
		}
		ord, err = em.exchange.OpenShort(price, amount, false, "")
		if err != nil {
			em.log.Warningln("SellShort 失败，正在retry")
			time.Sleep(orderCheckInterval * time.Millisecond)
			continue
		} else {
			state = 1
			em.log.Warningln("SellShort 挂单成功", price, amount)
			break
		}
	}
	if state == 0 {
		return false, nil
	}
	for ; count < totalOrderRetry; count++ {
		ord = utils.RE(em.exchange.GetOrder, ord.OrderID).(*bitmexwrap.Order)
		if ord.Status != "Filled" {
			time.Sleep(orderCheckInterval * time.Millisecond)
			continue
		} else {
			em.log.Warningln("SellShort 挂单已成交")
			return true, ord
		}
	}
	if state == 1 {
		if FORCE_ENTER {
			remainAmount := ord.Amount - ord.DealAmount
			em.log.Warningln("remainAmount:", remainAmount)
			if remainAmount > 0 { //处理头寸
				time.Sleep(orderCheckInterval * time.Millisecond)
				depth := utils.RE(em.exchange.GetDepth, 20).(bitmexwrap.Orderbook)
				var price float64
				price = em.takeWithBestPrice(remainAmount, depth.Buys) - SLIP
				ord, _ = em.exchange.OrderAmend(ord.OrderID, price)
			}
			em.log.Warningln("SellShort 挂单未成交, 增加滑点后成交")
		} else {
			utils.RE(em.exchange.CancelOrder, ord.OrderID)
			em.log.Warningln("SellShort 挂单未成交， 取消后退出")
		}
		return true, ord
	}
	return true, ord
}

func (em *EmaMacd) CloseLong(amount float64) (bool, *bitmexwrap.Order) {
	var putOrderRetry = 60 * (em.period / 5)
	var totalOrderRetry = 150 * (em.period / 5)

	var ord *bitmexwrap.Order
	var err error
	var state = 0
	var count = 0
	amount = utils.Float64Round(amount, num_n)
	for ; count < putOrderRetry; count++ {
		depth := utils.RE(em.exchange.GetDepth, 5).(bitmexwrap.Orderbook)
		var price float64
		if TAKE {
			price = utils.Float64Round(depth.Buys[0].Price-SLIP, price_n)
		} else {
			price = utils.Float64Round(depth.Sells[0].Price, price_n)
		}
		ord, err = em.exchange.CloseLong(price, amount, false, "")

		if err != nil {
			em.log.Warningln("CloseLong 失败，正在retry")
			time.Sleep(orderCheckInterval * time.Millisecond)
			continue
		} else {
			state = 1
			em.log.Warningln("CloseLong 挂单成功", price, amount)
			break
		}
	}
	if state == 0 {
		return false, nil
	}
	for ; count < totalOrderRetry; count++ {
		ord = utils.RE(em.exchange.GetOrder, ord.OrderID).(*bitmexwrap.Order)
		em.log.Debugln("ord status:", ord.Status)
		if ord.Status != "Filled" {
			time.Sleep(orderCheckInterval * time.Millisecond)
			continue
		} else {
			em.log.Warningln("CloseLong 挂单已成交")
			return true, ord
		}
	}
	if state == 1 {
		//em.log.Debugln("before cancel ord:", ord)
		//utils.RE(em.exchange.CancelOrder, ord.OrderID)
		//em.log.Debugln("after cancel ord:", ord)
		remainAmount := ord.Amount - ord.DealAmount
		em.log.Warningln("remainAmount:", remainAmount)
		if remainAmount > 0 { //处理头寸
			time.Sleep(orderCheckInterval * time.Millisecond)
			depth := utils.RE(em.exchange.GetDepth, 20).(bitmexwrap.Orderbook)
			var price float64
			price = em.takeWithBestPrice(remainAmount, depth.Buys) - SLIP
			//em.exchange.CloseShort(price, remainAmount, false, "")
			ord, _ = em.exchange.OrderAmend(ord.OrderID, price)
			//ord.DealAmount = ord.Amount
		}
		em.log.Warningln("CloseLong 挂单未成交， 取消后退出")
		return true, ord
	}
	return true, ord
}

func (em *EmaMacd) CloseShort(amount float64) (bool, *bitmexwrap.Order) {
	var putOrderRetry = 60 * (em.period / 5)
	var totalOrderRetry = 150 * (em.period / 5)

	var ord *bitmexwrap.Order
	var err error
	var state = 0
	var count = 0
	amount = utils.Float64Round(amount, num_n)
	for ; count < putOrderRetry; count++ {
		depth := utils.RE(em.exchange.GetDepth, 5).(bitmexwrap.Orderbook)
		var price float64
		if TAKE {
			price = utils.Float64Round(depth.Sells[0].Price+SLIP, price_n)
		} else {
			price = utils.Float64Round(depth.Buys[0].Price, price_n)
		}
		ord, err = em.exchange.CloseShort(price, amount, false, "")

		if err != nil {
			em.log.Warningln("CloseShort 失败，正在retry")
			time.Sleep(orderCheckInterval * time.Millisecond)
			continue
		} else {
			state = 1
			em.log.Warningln("CloseShort 挂单成功", price, amount)
			break
		}
	}
	if state == 0 {
		return false, nil
	}
	for ; count < totalOrderRetry; count++ {
		ord = utils.RE(em.exchange.GetOrder, ord.OrderID).(*bitmexwrap.Order)
		em.log.Debugln("ord status:", ord.Status)
		if ord.Status != "Filled" {
			time.Sleep(orderCheckInterval * time.Millisecond)
			continue
		} else {
			em.log.Warningln("CloseShort 挂单已成交")
			return true, ord
		}
	}
	if state == 1 {
		//em.log.Debugln("before cancel ord:", ord)
		//utils.RE(em.exchange.CancelOrder, ord.OrderID)
		//em.log.Debugln("after cancel ord:", ord)
		remainAmount := ord.Amount - ord.DealAmount
		em.log.Warningln("remainAmount:", remainAmount)
		if remainAmount > 0 { //处理头寸
			time.Sleep(orderCheckInterval * time.Millisecond)
			depth := utils.RE(em.exchange.GetDepth, 20).(bitmexwrap.Orderbook)
			var price float64
			price = em.takeWithBestPrice(remainAmount, depth.Sells) + SLIP
			//em.exchange.CloseShort(price, remainAmount, false, "")
			ord, _ = em.exchange.OrderAmend(ord.OrderID, price)
			//ord.DealAmount = ord.Amount
		}
		em.log.Warningln("CloseShort 挂单未成交， 取消后退出")
		return true, ord
	}
	return true, ord
}

func (em *EmaMacd) CancelAllPendingOrders() {
	for {
		time.Sleep(time.Second * 2)
		ord, err := em.exchange.CancelAllOrders()
		if err != nil {
			continue
		}
		pengding := false
		for _, v := range ord {
			if v.Status == "New" {
				pengding = true
				break
			}
		}
		if pengding {
			continue
		} else {
			break
		}

	}
}
