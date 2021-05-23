package okexapi

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	"github.com/c9s/bbgo/pkg/util"
	"github.com/pkg/errors"
)

const defaultHTTPTimeout = time.Second * 15
const RestBaseURL = "https://www.okex.com/"
const PublicWebSocketURL = "wss://ws.okex.com:8443/ws/v5/public"
const PrivateWebSocketURL = "wss://ws.okex.com:8443/ws/v5/private"

type RestClient struct {
	BaseURL *url.URL

	client *http.Client

	Key, Secret, Passphrase string
}

func NewClient() *RestClient {
	u, err := url.Parse(RestBaseURL)
	if err != nil {
		panic(err)
	}

	return &RestClient{
		BaseURL: u,
		client: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
	}
}

func (c *RestClient) Auth(key, secret, passphrase string) {
	c.Key = key
	c.Secret = secret
	c.Passphrase = passphrase
}

// NewRequest create new API request. Relative url can be provided in refURL.
func (c *RestClient) newRequest(method, refURL string, params url.Values, body []byte) (*http.Request, error) {
	rel, err := url.Parse(refURL)
	if err != nil {
		return nil, err
	}

	if params != nil {
		rel.RawQuery = params.Encode()
	}

	pathURL := c.BaseURL.ResolveReference(rel)

	req, err := http.NewRequest(method, pathURL.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	return req, nil
}

// newAuthenticatedRequest creates new http request for authenticated routes.
func (c *RestClient) newAuthenticatedRequest(method, refURL string, params url.Values) (*http.Request, error) {
	if len(c.Key) == 0 {
		return nil, errors.New("empty api key")
	}

	if len(c.Secret) == 0 {
		return nil, errors.New("empty api secret")
	}

	rel, err := url.Parse(refURL)
	if err != nil {
		return nil, err
	}

	if params != nil {
		rel.RawQuery = params.Encode()
	}

	pathURL := c.BaseURL.ResolveReference(rel)
	path := pathURL.Path

	// set location to UTC so that it outputs "2020-12-08T09:08:57.715Z"
	t := time.Now().In(time.UTC)
	timestamp := t.Format("2006-01-02T15:04:05.999Z07:00")

	payload := timestamp + strings.ToUpper(method) + path
	sign := signPayload(payload, c.Secret)

	var body []byte
	req, err := http.NewRequest(method, pathURL.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("OK-ACCESS-KEY", c.Key)
	req.Header.Add("OK-ACCESS-SIGN", sign)
	req.Header.Add("OK-ACCESS-TIMESTAMP", timestamp)
	req.Header.Add("OK-ACCESS-PASSPHRASE", c.Passphrase)

	return req, nil
}

type BalanceDetail struct {
	Currency                string `json:"ccy"`
	Available               string `json:"availEq"`
	CashBalance             string `json:"cashBal"`
	OrderFrozen             string `json:"ordFrozen"`
	Frozen                  string `json:"frozenBal"`
	Equity                  string `json:"eq"`
	EquityInUSD             string `json:"eqUsd"`
	UpdateTime              string `json:"uTime"`
	UnrealizedProfitAndLoss string `json:"upl"`
}

type BalanceSummary struct {
	TotalEquityInUSD string          `json:"totalEq"`
	UpdateTime       string          `json:"uTime"`
	Details          []BalanceDetail `json:"details"`
}

type BalanceSummaryList []BalanceSummary

func (c *RestClient) AccountBalances() (BalanceSummaryList, error) {
	req, err := c.newAuthenticatedRequest("GET", "/api/v5/account/balance", nil)
	if err != nil {
		return nil, err
	}

	response, err := c.sendRequest(req)
	if err != nil {
		return nil, err
	}

	var balanceResponse struct {
		Code    string           `json:"code"`
		Message string           `json:"msg"`
		Data    []BalanceSummary `json:"data"`
	}
	if err := response.DecodeJSON(&balanceResponse); err != nil {
		return nil, err
	}

	return balanceResponse.Data, nil
}

type AssetBalance struct {
	Currency  string `json:"ccy"`
	Balance   string `json:"bal"`
	Frozen    string `json:"frozenBal,omitempty"`
	Available string `json:"availBal,omitempty"`
}

type AssetBalanceList []AssetBalance

func (c *RestClient) AssetBalances() (AssetBalanceList, error) {
	req, err := c.newAuthenticatedRequest("GET", "/api/v5/asset/balances", nil)
	if err != nil {
		return nil, err
	}

	response, err := c.sendRequest(req)
	if err != nil {
		return nil, err
	}

	var balanceResponse struct {
		Code    string           `json:"code"`
		Message string           `json:"msg"`
		Data    AssetBalanceList `json:"data"`
	}
	if err := response.DecodeJSON(&balanceResponse); err != nil {
		return nil, err
	}

	return balanceResponse.Data, nil
}

type AssetCurrency struct {
	Currency               string `json:"ccy"`
	Name                   string `json:"name"`
	Chain                  string `json:"chain"`
	CanDeposit             bool   `json:"canDep"`
	CanWithdraw            bool   `json:"canWd"`
	CanInternal            bool   `json:"canInternal"`
	MinWithdrawalFee       string `json:"minFee"`
	MaxWithdrawalFee       string `json:"maxFee"`
	MinWithdrawalThreshold string `json:"minWd"`
}

func (c *RestClient) AssetCurrencies() ([]AssetCurrency, error) {
	req, err := c.newAuthenticatedRequest("GET", "/api/v5/asset/currencies", nil)
	if err != nil {
		return nil, err
	}

	response, err := c.sendRequest(req)
	if err != nil {
		return nil, err
	}

	var currencyResponse struct {
		Code    string          `json:"code"`
		Message string          `json:"msg"`
		Data    []AssetCurrency `json:"data"`
	}

	if err := response.DecodeJSON(&currencyResponse); err != nil {
		return nil, err
	}

	return currencyResponse.Data, nil
}

type MarketTicker struct {
	InstrumentType string `json:"instType"`
	InstrumentID   string `json:"instId"`

	// last traded price
	Last fixedpoint.Value `json:"last"`

	// last traded size
	LastSize fixedpoint.Value `json:"lastSz"`

	AskPrice fixedpoint.Value `json:"askPx"`
	AskSize  fixedpoint.Value `json:"askSz"`

	BidPrice fixedpoint.Value `json:"bidPx"`
	BidSize  fixedpoint.Value `json:"bidSz"`

	Open24H           fixedpoint.Value `json:"open24h"`
	High24H           fixedpoint.Value `json:"high24H"`
	Low24H            fixedpoint.Value `json:"low24H"`
	Volume24H         fixedpoint.Value `json:"vol24h"`
	VolumeCurrency24H fixedpoint.Value `json:"volCcy24h"`

	// Millisecond timestamp
	Timestamp types.MillisecondTimestamp `json:"ts"`
}

func (c *RestClient) MarketTicker(instId string) (*MarketTicker, error) {
	// SPOT, SWAP, FUTURES, OPTION
	var params = url.Values{}
	params.Add("instId", instId)

	req, err := c.newAuthenticatedRequest("GET", "/api/v5/market/ticker", params)
	if err != nil {
		return nil, err
	}

	response, err := c.sendRequest(req)
	if err != nil {
		return nil, err
	}

	var tickerResponse struct {
		Code    string         `json:"code"`
		Message string         `json:"msg"`
		Data    []MarketTicker `json:"data"`
	}
	if err := response.DecodeJSON(&tickerResponse); err != nil {
		return nil, err
	}

	if len(tickerResponse.Data) == 0 {
		return nil, fmt.Errorf("ticker of %s not found", instId)
	}

	return &tickerResponse.Data[0], nil
}

func (c *RestClient) MarketTickers(instType string) ([]MarketTicker, error) {
	// SPOT, SWAP, FUTURES, OPTION
	var params = url.Values{}
	params.Add("instType", instType)

	req, err := c.newAuthenticatedRequest("GET", "/api/v5/market/tickers", params)
	if err != nil {
		return nil, err
	}

	response, err := c.sendRequest(req)
	if err != nil {
		return nil, err
	}

	var tickerResponse struct {
		Code    string         `json:"code"`
		Message string         `json:"msg"`
		Data    []MarketTicker `json:"data"`
	}
	if err := response.DecodeJSON(&tickerResponse); err != nil {
		return nil, err
	}

	return tickerResponse.Data, nil
}

// sendRequest sends the request to the API server and handle the response
func (c *RestClient) sendRequest(req *http.Request) (*util.Response, error) {
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	// newResponse reads the response body and return a new Response object
	response, err := util.NewResponse(resp)
	if err != nil {
		return response, err
	}

	// Check error, if there is an error, return the ErrorResponse struct type
	if response.IsError() {
		return response, errors.New(string(response.Body))
	}

	return response, nil
}

func signPayload(payload string, secret string) string {
	var sig = hmac.New(sha256.New, []byte(secret))
	_, err := sig.Write([]byte(payload))
	if err != nil {
		return ""
	}

	return base64.StdEncoding.EncodeToString(sig.Sum(nil))
	// return hex.EncodeToString(sig.Sum(nil))
}
