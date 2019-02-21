package trade

import (
	"github.com/beaquant/utils"
	"github.com/nntaoli-project/GoEx"
	"github.com/sirupsen/logrus"
	"time"
)

type FutureTradeManager struct {
	exchange     goex.FutureRestAPI //交易所
	pair         goex.CurrencyPair  //货币对
	contractType string             //合约类型
	initAccount  *Account           //初始账户
	//opMode       OpMode            //下单方式:吃单|挂单
	//maxSpace     float64           //挂单失效距离:
	slidePrice                      float64 //下单滑动价
	slideGrowthRate                 float64 //下单滑动价增长率
	openPositionSlideGrowthRateMax  float64 //下单滑动价最大增长率
	coverPositionSlideGrowthRateMax float64 //下单滑动价最大增长率
	//maxAmount    float64           //开仓最大单次下单量
	//minStocks    float64           //最小交易数量
	retryDelayMs time.Duration  //失败重试(毫秒)
	logger       *logrus.Logger //logger
	priceDot     int            //价格小数精度
	amountDot    int            //数量小数精度
	marginLevel  int            //杆杠大小
}

type SummaryPosition struct {
	Amount   float64   //持仓量, OKCoin表示合约的份数(整数且大于1)
	Price    float64   //持仓均价
	Position *Position //
}

type Position struct {
	MarginLevel  int     //杆杠大小, OKCoin为10或者20。
	Amount       float64 //持仓量, OKCoin表示合约的份数(整数且大于1)
	FrozenAmount float64 //仓位冻结量
	Price        float64 //持仓均价
	Profit       float64 //持仓浮动盈亏(数据货币单位：BTC/LTC, 传统期货单位:RMB, 股票不支持此字段, 注: OKCoin期货全仓情况下指实现盈余, 并非持仓盈亏, 逐仓下指持仓盈亏)
	Type         int     //PD_LONG为多头仓位(CTP中用closebuy_today平仓), PD_SHORT为空头仓位(CTP用closesell_today)平仓, (CTP期货中)PD_LONG_YD为咋日多头仓位(用closebuy平), PD_SHORT_YD为咋日空头仓位(用closesell平)
	ContractType string  //商品期货为合约代码
}

func NewFutureTradeManager(
	exchange goex.FutureRestAPI,
	pair goex.CurrencyPair,
	contractType string,
	//opMode OpMode,
	//maxSpace float64,
	slidePrice float64,
	openPositionSlideGrowthRateMax float64,
	coverPositionSlideGrowthRateMax float64,
	//maxAmount float64,
	//minStocks float64,
	retryDelayMs int,
	logger *logrus.Logger,
	priceDot int,
	amountDot int,
) *FutureTradeManager {
	if logger == nil {
		logger = logrus.New()
	}
	utils.SetDelay(retryDelayMs)
	mgr := &FutureTradeManager{
		exchange:     exchange,
		pair:         pair,
		contractType: contractType,
		initAccount:  nil,
		//opMode:       opMode,
		//maxAmount:    maxAmount,
		//maxSpace:     maxSpace,
		slidePrice:                      slidePrice,
		openPositionSlideGrowthRateMax:  openPositionSlideGrowthRateMax,
		coverPositionSlideGrowthRateMax: coverPositionSlideGrowthRateMax,
		//minStocks:    minStocks,
		retryDelayMs: time.Duration(retryDelayMs) * time.Millisecond,
		logger:       logger,
		priceDot:     priceDot,
		amountDot:    amountDot,
	}
	mgr.initAccount = mgr.GetAccount()
	return mgr
}

func (future *FutureTradeManager) getPosition(direction int) *Position {

	var allCost = 0.0
	var allAmount = 0.0
	var allProfit = 0.0
	var allFrozen = 0.0
	var posMargin = 0
	var positions = utils.RE(future.exchange.GetFuturePosition, future.pair, future.contractType).([]goex.FuturePosition)
	if len(positions) == 0 {
		return nil
	}

	for i := 0; i < len(positions); i++ {
		if positions[i].ContractType == future.contractType {
			posMargin = positions[i].LeverRate
			if direction == goex.OPEN_BUY {
				allCost += positions[i].BuyPriceAvg * positions[i].BuyAmount
				allAmount += positions[i].BuyAmount
				allProfit += positions[i].BuyProfitReal
				allFrozen += positions[i].BuyAmount - positions[i].BuyAvailable
			} else if direction == goex.OPEN_SELL {
				allCost += positions[i].BuyPriceAvg * positions[i].SellAmount
				allAmount += positions[i].SellAmount
				allProfit += positions[i].SellProfitReal
				allFrozen += positions[i].SellAmount - positions[i].SellAvailable
			}
		}
	}
	if allAmount == 0 {
		return nil
	}
	return &Position{
		MarginLevel:  posMargin,
		FrozenAmount: allFrozen,
		Price:        utils.Float64Round(allCost/allAmount, future.priceDot),
		Amount:       allAmount,
		Profit:       allProfit,
		Type:         direction,
		ContractType: future.contractType,
	}
}

// direction : goex.OPEN_BUY, goex.OPEN_SELL
func (future *FutureTradeManager) open(direction int, price, opAmount float64) *SummaryPosition {
	var initPosition = future.getPosition(direction)
	var isFirst = true
	var initAmount = 0.0
	var positionNow = initPosition
	var step = 0.0
	if initPosition != nil {
		initAmount = initPosition.Amount
	}
	for {
		var needOpen = opAmount
		if isFirst {
			isFirst = false
		} else {
			positionNow = future.getPosition(direction)
			if positionNow != nil {
				needOpen = opAmount - (positionNow.Amount - initAmount)
			}
		}
		if needOpen < 1 {
			break
		}
		if step > future.openPositionSlideGrowthRateMax {
			break
		}
		var amount = needOpen
		//var orderId string
		if direction == goex.OPEN_BUY {
			future.exchange.PlaceFutureOrder(
				future.pair,
				future.contractType,
				utils.Float64RoundString(price+future.slidePrice*(1+future.slideGrowthRate), future.priceDot),
				utils.Float64RoundString(amount, future.amountDot),
				goex.OPEN_BUY,
				0,
				future.marginLevel,
			)
		} else if direction == goex.OPEN_SELL {
			future.exchange.PlaceFutureOrder(
				future.pair,
				future.contractType,
				utils.Float64RoundString(price-future.slidePrice*(1+future.slideGrowthRate), future.priceDot),
				utils.Float64RoundString(amount, future.amountDot),
				goex.OPEN_SELL,
				0,
				future.marginLevel,
			)
		}
		for {
			var orders = utils.RE(future.exchange.GetUnfinishFutureOrders, future.pair, future.contractType).([]goex.FutureOrder)
			if len(orders) == 0 {
				break
			}
			time.Sleep(future.retryDelayMs)
			for j := 0; j < len(orders); j++ {
				future.exchange.FutureCancelOrder(future.pair, future.contractType, orders[j].OrderID2)
				if j < (len(orders) - 1) {
					time.Sleep(future.retryDelayMs)
				}
			}
		}
		step += future.slideGrowthRate
	}
	var pos = &SummaryPosition{
		Price:    0,
		Amount:   0,
		Position: positionNow,
	}
	if positionNow == nil {
		return pos
	}
	if initPosition == nil {
		pos.Price = positionNow.Price
		pos.Amount = positionNow.Amount
	} else {
		pos.Amount = positionNow.Amount - initPosition.Amount
		pos.Price = utils.Float64Round(((positionNow.Price*positionNow.Amount)-(initPosition.Price*initPosition.Amount))/pos.Amount, future.priceDot)
	}
	return pos
}

//
// direction : goex.CLOSE_BUY, goex.CLOSE_SELL
func (future *FutureTradeManager) cover(direction int, opAmount, price float64) float64 {
	var initP = make([]goex.FuturePosition, 0)
	var positions = make([]goex.FuturePosition, 0)
	var isFirst = true
	var orderId string
	var err error
	var step = 0.0
	var index = 0
	for {
		var n = 0
		positions = utils.RE(future.exchange.GetFuturePosition, future.pair, future.contractType).([]goex.FuturePosition)
		if isFirst == true {
			if len(positions) > 1 || (direction != goex.CLOSE_BUY && direction != goex.CLOSE_SELL) {
				future.logger.Fatalln("有多，空双向持仓，并且参数direction未明确方向！或 direction 参数异常：", direction)
			}
			copy(initP, positions)
			isFirst = false
		}
		for i := 0; i < len(positions); i++ {
			if positions[i].ContractType != future.contractType ||
				(positions[i].BuyAmount == 0 && direction == goex.CLOSE_BUY) ||
				(positions[i].SellAmount == 0 && direction == goex.CLOSE_SELL) {
				continue
			}
			var amount = 0.0
			if direction == goex.CLOSE_BUY {
				amount = opAmount - (initP[i].BuyAmount - positions[i].BuyAmount)
			} else if direction == goex.CLOSE_BUY {
				amount = opAmount - (initP[i].SellAmount - positions[i].SellAmount)
			}

			if amount == 0 {
				continue
			}
			if direction == goex.CLOSE_BUY {
				orderId, err = future.exchange.PlaceFutureOrder(
					future.pair,
					future.contractType,
					utils.Float64RoundString(price-future.slidePrice*(1+step), future.priceDot),
					utils.Float64RoundString(amount, future.amountDot),
					goex.CLOSE_BUY,
					0,
					future.marginLevel,
				)
				n++
			} else if direction == goex.CLOSE_SELL {
				orderId, err = future.exchange.PlaceFutureOrder(
					future.pair,
					future.contractType,
					utils.Float64RoundString(price+future.slidePrice*(1+step), future.priceDot),
					utils.Float64RoundString(amount, future.amountDot),
					goex.CLOSE_SELL,
					0,
					future.marginLevel,
				)
				n++
			}
			index = i
		}
		if n == 0 {
			break
		}
		time.Sleep(future.retryDelayMs)

		future.exchange.FutureCancelOrder(future.pair, future.contractType, orderId)
		step += future.slideGrowthRate
		if step > future.coverPositionSlideGrowthRateMax {
			break
		}
	}

	var nowP = utils.RE(future.exchange.GetFuturePosition, future.pair, future.contractType).([]goex.FuturePosition)
	if len(nowP)-1 < index ||
		(nowP[index].BuyAmount != initP[index].BuyAmount && direction == goex.CLOSE_BUY) ||
		(nowP[index].SellAmount != initP[index].SellAmount && direction == goex.CLOSE_SELL) {
		if len(initP) == 0 {
			return 0
		} else {
			if direction == goex.CLOSE_BUY {
				return initP[index].BuyAmount
			} else if direction == goex.CLOSE_SELL {
				return initP[index].SellAmount
			}
		}
	} else {
		if direction == goex.CLOSE_BUY {
			return initP[index].BuyAmount - nowP[index].BuyAmount
		} else if direction == goex.CLOSE_SELL {
			return initP[index].SellAmount - nowP[index].SellAmount
		}
	}
	return 0
}

func (future *FutureTradeManager) GetAccount() *Account {
	var account = new(Account)
	for {
		acc := utils.RE(future.exchange.GetFutureUserinfo).(*goex.FutureAccount)
		for _, v := range acc.FutureSubAccounts {
			if v.Currency == future.pair.CurrencyB {
				account.Balance = v.KeepDeposit
				account.FrozenBalance = v.KeepDeposit - v.AccountRights
			} else if v.Currency == future.pair.CurrencyA {
				account.Balance = v.KeepDeposit
				account.FrozenBalance = v.KeepDeposit - v.AccountRights
			}
		}
	}
	return account
}

func (future *FutureTradeManager) OpenLong(price, opAmount float64) *SummaryPosition {
	return future.open(goex.OPEN_BUY, opAmount, price)
}

func (future *FutureTradeManager) OpenShort(price, opAmount float64) *SummaryPosition {
	return future.open(goex.OPEN_SELL, opAmount, price)
}

func (future *FutureTradeManager) CloseLong(price, opAmount float64) float64 {
	return future.cover(goex.CLOSE_BUY, opAmount, price)
}

func (future *FutureTradeManager) CloseShort(price, opAmount float64) float64 {
	return future.cover(goex.CLOSE_SELL, opAmount, price)
}

func (future *FutureTradeManager) Profit(price, opAmount float64) float64 {
	var accountNow = future.GetAccount()
	future.logger.Infoln("NOW:", accountNow, "--account:", future.initAccount)
	return utils.Float64Round(accountNow.Balance - future.initAccount.Balance)
}
