package trade

import (
	"github.com/beaquant/utils"
	"github.com/nntaoli-project/GoEx"
	"github.com/sirupsen/logrus"
	"math"
	"time"
)

type SpotTradeManager struct {
	exchange     goex.API          //交易所
	pair         goex.CurrencyPair //货币对
	opMode       OpMode            //下单方式:吃单|挂单|挂单等待
	maxSpace     float64           //挂单失效距离
	slidePrice   float64           //下单滑动价
	maxAmount    float64           //开仓最大单次下单量
	minStocks    float64           //最小交易数量
	retryDelayMs time.Duration     //失败重试(毫秒)
	waitMakeMs   int               //失败重试(毫秒)
	logger       *logrus.Logger    //logger
	priceDot     int               //价格小数精度
	amountDot    int               //数量小数精度
}

type OpMode int

const (
	OPMODE_TAKE = 1 + iota
	OPMODE_MAKE
	OPMODE_MAKE_WAIT
)

func (op OpMode) String() string {
	switch op {
	case OPMODE_TAKE:
		return "OPMODE_TAKE"
	case OPMODE_MAKE:
		return "OPMODE_MAKE"
	case OPMODE_MAKE_WAIT:
		return "OPMODE_MAKE_WAIT"
	default:
		return "UNKNOWN"
	}
}

type Account struct {
	Pair          goex.CurrencyPair `json:"pair"`
	Balance       float64           `json:"balance"`
	FrozenBalance float64           `json:"frozen_balance"`
	Stocks        float64           `json:"stocks"`
	FrozenStocks  float64           `json:"frozen_stocks"`
}

func NewSportManager(
	exchange goex.API,
	pair goex.CurrencyPair,
	opMode OpMode,
	maxSpace float64,
	slidePrice float64,
	maxAmount float64,
	minStocks float64,
	retryDelayMs int,
	waitMakeMs int,
	logger *logrus.Logger,
	priceDot int,
	amountDot int,
) *SpotTradeManager {
	if logger == nil {
		logger = logrus.New()
	}
	utils.SetDelay(retryDelayMs)
	return &SpotTradeManager{
		exchange:     exchange,
		pair:         pair,
		opMode:       opMode,
		maxAmount:    maxAmount,
		maxSpace:     maxSpace,
		slidePrice:   slidePrice,
		minStocks:    minStocks,
		retryDelayMs: time.Duration(retryDelayMs) * time.Millisecond,
		waitMakeMs:   waitMakeMs,
		logger:       logger,
		priceDot:     priceDot,
		amountDot:    amountDot,
	}
}

func (spot *SpotTradeManager) CancelPendingOrders(orderType goex.TradeSide) {
	for {
		orders := utils.RE(spot.exchange.GetUnfinishOrders, spot.pair).([]goex.Order)
		if len(orders) == 0 {
			break
		}
		for j := 0; j < len(orders); j++ {
			if orders[j].Side != orderType {
				continue
			}
			spot.exchange.CancelOrder(orders[j].OrderID2, spot.pair)
			if j < len(orders)-1 {
				time.Sleep(spot.retryDelayMs)
			}
		}
	}
}

func (spot *SpotTradeManager) CancelAllPendingOrders() {
	for {
		orders := utils.RE(spot.exchange.GetUnfinishOrders, spot.pair).([]goex.Order)
		if len(orders) == 0 {
			break
		}
		for j := 0; j < len(orders); j++ {
			spot.exchange.CancelOrder(orders[j].OrderID2, spot.pair)
			if j < len(orders)-1 {
				time.Sleep(spot.retryDelayMs)
			}
		}
	}
}

func (spot *SpotTradeManager) StripOrders(orderId string) *goex.Order {
	var order = new(goex.Order)
	if orderId == "" {
		spot.CancelAllPendingOrders()
	}
	for {
		orders := utils.RE(spot.exchange.GetUnfinishOrders, spot.pair).([]goex.Order)
		if len(orders) == 0 {
			break
		}
		var dropped = 0
		for j := 0; j < len(orders); j++ {
			if orders[j].OrderID2 == orderId {
				order = &orders[j]
			} else {
				spot.exchange.CancelOrder(orders[j].OrderID2, spot.pair)
				dropped++
				if j < len(orders)-1 {
					time.Sleep(spot.retryDelayMs)
				}
			}
		}
		if dropped == 0 {
			break
		}
	}
	return order
}

func (spot *SpotTradeManager) GetAccount(waitFrozen bool) *Account {
	var account = new(Account)
	var alreadyAlert = false
	for {
		acc := utils.RE(spot.exchange.GetAccount).(*goex.Account)
		for _, v := range acc.SubAccounts {
			if v.Currency == spot.pair.CurrencyB {
				account.Balance = v.Amount
				account.FrozenBalance = v.ForzenAmount
			} else if v.Currency == spot.pair.CurrencyA {
				account.Stocks = v.Amount
				account.FrozenStocks = v.ForzenAmount
			}
		}

		if !waitFrozen || (account.FrozenStocks < spot.minStocks && account.FrozenBalance < 0.01) {
			break
		}
		if !alreadyAlert {
			alreadyAlert = true
			spot.logger.Infoln("发现账户有冻结的钱或币", account)
		}
		time.Sleep(spot.retryDelayMs)
	}
	return account
}

func (spot *SpotTradeManager) tradeFunc(tradeType goex.TradeSide) (func(amount, price string, currency goex.CurrencyPair) (*goex.Order, error), bool) {
	switch tradeType {
	case goex.BUY:
		return spot.exchange.LimitBuy, true
	case goex.SELL:
		return spot.exchange.LimitSell, false
	case goex.BUY_MARKET:
		return spot.exchange.MarketBuy, true
	case goex.SELL_MARKET:
		return spot.exchange.MarketSell, false
	default:
		spot.logger.Fatalln("UNKNOWN tradeType")
	}
	panic("UNKNOWN tradeType")
}

func (spot *SpotTradeManager) trade(opMode OpMode, tradeType goex.TradeSide, tradeAmount float64) *goex.Order {
	var initAccount = spot.GetAccount(false)
	var nowAccount = initAccount
	var order = new(goex.Order)
	var prePrice = 0.0
	var firstPrice = 0.0
	var dealAmount = 0.0
	var diffMoney = 0.0
	var isFirst = true
	var err error
	var tradeFunc, isBuy = spot.tradeFunc(tradeType)
	for {
		var ticker = utils.RE(spot.exchange.GetTicker, spot.pair).(*goex.Ticker)
		var tradePrice = 0.0
		if isBuy {
			if opMode == OPMODE_TAKE {
				tradePrice = utils.Float64Round(ticker.Sell+spot.slidePrice, spot.priceDot)
			} else if opMode == OPMODE_MAKE {
				tradePrice = utils.Float64Round(ticker.Buy+spot.slidePrice, spot.priceDot)
			} else if opMode == OPMODE_MAKE_WAIT {
				tradePrice = utils.Float64Round(ticker.Buy, spot.priceDot)
			}
		} else {
			if opMode == OPMODE_TAKE {
				tradePrice = utils.Float64Round(ticker.Buy-spot.slidePrice, spot.priceDot)
			} else if opMode == OPMODE_MAKE {
				tradePrice = utils.Float64Round(ticker.Sell-spot.slidePrice, spot.priceDot)
			} else if opMode == OPMODE_MAKE_WAIT {
				tradePrice = utils.Float64Round(ticker.Sell, spot.priceDot)
			}
		}
		if opMode == OPMODE_MAKE_WAIT { //if make_wait fail, change to make
			for wait := 0; wait < spot.waitMakeMs/int(spot.retryDelayMs.Nanoseconds()/time.Millisecond.Nanoseconds()); wait++ {
				order, err = tradeFunc(utils.Float64RoundString(tradeAmount, spot.amountDot), utils.Float64RoundString(tradePrice, spot.priceDot), spot.pair)
				if err != nil {
					time.Sleep(spot.retryDelayMs)
					continue
				}
				for ; wait < spot.waitMakeMs/int(spot.retryDelayMs.Nanoseconds()/time.Millisecond.Nanoseconds()); wait++ {
					order = utils.RE(spot.exchange.GetOneOrder, order.OrderID2, spot.pair).(*goex.Order)
					if order.Status == goex.ORDER_FINISH {
						return order
					} else {
						time.Sleep(spot.retryDelayMs)
						continue
					}
				}
				if wait >= spot.waitMakeMs/int(spot.retryDelayMs.Nanoseconds()/time.Millisecond.Nanoseconds()) && order.Status != goex.ORDER_FINISH {
					utils.RE(spot.exchange.CancelOrder, order.OrderID2, spot.pair)
					return spot.trade(OPMODE_MAKE, tradeType, tradeAmount-order.DealAmount) //递归
				}
			}
		}

		if order == nil {
			if isFirst {
				isFirst = false
				firstPrice = tradePrice
			} else {
				nowAccount = spot.GetAccount(false)
			}
			var doAmount = 0.0
			if isBuy {
				diffMoney = utils.Float64Round(initAccount.Balance-nowAccount.Balance, 4)
				dealAmount = utils.Float64Round(nowAccount.Stocks-initAccount.Stocks, 8) // 如果保留小数过少，会引起在小交易量交易时，计算出的成交价格误差较大。
				doAmount = math.Min(math.Min(spot.maxAmount, tradeAmount-dealAmount), utils.Float64Round((nowAccount.Balance*0.95)/tradePrice, 4))
			} else {
				diffMoney = utils.Float64Round(nowAccount.Balance-initAccount.Balance, 4)
				dealAmount = utils.Float64Round(initAccount.Stocks-nowAccount.Stocks, 8)
				doAmount = math.Min(math.Min(spot.maxAmount, tradeAmount-dealAmount), nowAccount.Stocks)
			}
			if doAmount < spot.minStocks {
				break
			}
			prePrice = tradePrice
			order, err = tradeFunc(utils.Float64RoundString(doAmount, spot.amountDot), utils.Float64RoundString(tradePrice, spot.priceDot), spot.pair)
			if err != nil {
				spot.CancelPendingOrders(tradeType)
			}
		} else {
			if opMode == OPMODE_TAKE || (math.Abs(tradePrice-prePrice) > spot.maxSpace) {
				order = nil
				spot.CancelAllPendingOrders()
			} else {
				var ord = spot.StripOrders(order.OrderID2)
				if ord == nil {
					order = nil
				}
			}
		}
		time.Sleep(spot.retryDelayMs)
	}
	if dealAmount <= 0 {
		return nil
	}
	return &goex.Order{
		Side:       tradeType,
		Currency:   spot.pair,
		Price:      firstPrice,
		Amount:     tradeAmount,
		AvgPrice:   utils.Float64Round(diffMoney/dealAmount, spot.priceDot),
		DealAmount: utils.Float64Round(dealAmount, spot.amountDot),
	}
}

func (spot *SpotTradeManager) Buy(amount float64) {
	spot.trade(spot.opMode, goex.BUY, amount)
}

func (spot *SpotTradeManager) Sell(amount float64) {
	spot.trade(spot.opMode, goex.SELL, amount)
}
