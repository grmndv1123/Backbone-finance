package bbgo

import (
	log "github.com/sirupsen/logrus"
	"time"
)

func CalculateAverageCost(trades []Trade) (averageCost float64) {
	var totalCost = 0.0
	var totalQuantity = 0.0
	for _, t := range trades {
		if t.IsBuyer {
			totalCost += t.Price * t.Volume
			totalQuantity += t.Volume
		} else {
			totalCost -= t.Price * t.Volume
			totalQuantity -= t.Volume
		}
	}

	averageCost = totalCost / totalQuantity
	return
}

type ProfitAndLossCalculator struct {
	Symbol       string
	StartTime    time.Time
	CurrentPrice float64
	Trades       []Trade
}

func (c *ProfitAndLossCalculator) AddTrade(trade Trade) {
	c.Trades = append(c.Trades, trade)
}

func (c *ProfitAndLossCalculator) SetCurrentPrice(price float64) {
	c.CurrentPrice = price
}

func (c *ProfitAndLossCalculator) Calculate() *ProfitAndLossReport {
	// copy trades, so that we can truncate it.
	var trades = c.Trades

	var bidVolume = 0.0
	var bidAmount = 0.0
	var bidFee = 0.0

	// find the first buy trade
	var firstBidIndex = -1
	for idx, t := range trades {
		if t.IsBuyer {
			firstBidIndex = idx
			break
		}
	}
	if firstBidIndex > 0 {
		trades = trades[firstBidIndex:]
	}

	for _, t := range trades {
		if t.IsBuyer {
			bidVolume += t.Volume
			bidAmount += t.Price * t.Volume
			switch t.FeeCurrency {
			case "BTC":
				bidFee += t.Price * t.Fee
			}
		}
	}

	log.Infof("average bid price = (total amount %f + total fee %f) / volume %f", bidAmount, bidFee, bidVolume)
	profit := 0.0
	averageBidPrice := (bidAmount + bidFee) / bidVolume

	var feeRate = 0.001
	var askVolume = 0.0
	var askFee = 0.0
	for _, t := range trades {
		if !t.IsBuyer {
			profit += (t.Price - averageBidPrice) * t.Volume
			askVolume += t.Volume
			switch t.FeeCurrency {
			case "USDT":
				askFee += t.Fee
			}
		}
	}

	profit -= askFee

	stock := bidVolume - askVolume
	futureFee := 0.0
	if stock > 0 {
		stockFee := c.CurrentPrice * feeRate * stock
		profit += (c.CurrentPrice-averageBidPrice)*stock - stockFee
		futureFee += stockFee
	}

	fee := bidFee + askFee + futureFee

	return &ProfitAndLossReport{
		CurrentPrice:    c.CurrentPrice,
		StartTime:       c.StartTime,
		NumTrades:       len(trades),

		Profit:          profit,
		AverageBidPrice: averageBidPrice,
		Stock:           stock,
		Fee:             fee,
	}
}

type ProfitAndLossReport struct {
	CurrentPrice float64
	StartTime    time.Time

	NumTrades       int
	Profit          float64
	AverageBidPrice float64
	Stock           float64
	Fee             float64
}

func (report ProfitAndLossReport) Print() {
	log.Infof("trades since: %v", report.StartTime)
	log.Infof("average bid price: %s", USD.FormatMoneyFloat64(report.AverageBidPrice))
	log.Infof("Stock volume: %f", report.Stock)
	log.Infof("current price: %s", USD.FormatMoneyFloat64(report.CurrentPrice))
	log.Infof("overall profit: %s", USD.FormatMoneyFloat64(report.Profit))
}

func CalculateCostAndProfit(trades []Trade, currentPrice float64, startTime time.Time) (report *ProfitAndLossReport) {
}
