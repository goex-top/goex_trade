package trade

import (
	"github.com/nntaoli-project/GoEx"
	"github.com/nntaoli-project/GoEx/builder"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"
)

var httpProxyClient = &http.Client{
	Transport: &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return &url.URL{
				Scheme: "socks5",
				Host:   "127.0.0.1:1080"}, nil
		},
		Dial: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).Dial,
	},
	Timeout: 10 * time.Second,
}

var futureExchange = builder.NewCustomAPIBuilder(httpProxyClient).APIKey("xxxx").APISecretkey("xxxx").FutureBuild(goex.BITMEX)
var futureMgr = NewFutureTradeManager(
	futureExchange,
	goex.NewCurrencyPair2("XBT_USD"),
	"",
	OPMODE_MAKE,
	1.0,
	0.01,
	0.05,
	0.05,
	500,
	nil,
	2,
	1,
)

func TestNewFutureTradeManager(t *testing.T) {
	t.Log(futureExchange.GetExchangeName())
}
func TestNewFutureTradeManager2(t *testing.T) {
	t.Log(futureExchange.GetFuturePosition(futureMgr.pair, ""))
	t.Log(futureMgr.getPosition(goex.SELL))
}

func TestFutureTradeManager_GetAccount(t *testing.T) {
	t.Log(futureMgr.GetAccount())

}

func TestFutureTradeManager_Profit(t *testing.T) {
	t.Log(futureMgr.Profit(10, 10))
}
