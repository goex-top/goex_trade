package trade

import "github.com/nntaoli-project/GoEx"

type SpotTradeManagerAPI interface {
	CancelPendingOrders(orderType goex.TradeSide)
	CancelAllPendingOrders()
	StripOrders(orderId string) *goex.Order
	GetAccount(waitFrozen bool) *Account
	Buy(amount float64) *goex.Order
	Sell(amount float64) *goex.Order
}
