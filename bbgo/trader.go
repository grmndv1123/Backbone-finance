package bbgo

import (
	"context"
	slack2 "github.com/c9s/bbgo/pkg/slack"
	log "github.com/sirupsen/logrus"
	"github.com/slack-go/slack"

	"github.com/c9s/bbgo/pkg/bbgo/exchange/binance"
	"github.com/c9s/bbgo/pkg/bbgo/types"
)

type Trader struct {
	Notifier *slack2.SlackNotifier

	// Context is trading Context
	Context *TradingContext

	Exchange *binance.Exchange

	Slack *slack.Client

	TradingChannel string
	ErrorChannel   string
	InfoChannel    string
}

func (t *Trader) Infof(format string, args ...interface{}) {
	t.Notifier.Infof(format, args...)
}

func (t *Trader) ReportTrade(trade *types.Trade) {
	t.Notifier.ReportTrade(trade)
}

func (t *Trader) ReportPnL() {
	report := t.Context.ProfitAndLossCalculator.Calculate()
	report.Print()
	t.Notifier.ReportPnL(report)
}

func (t *Trader) SubmitOrder(ctx context.Context, order *types.Order) {
	t.Infof(":memo: Submitting %s order on side %s with volume: %s", order.Type, order.Side, order.VolumeStr, order.SlackAttachment())

	err := t.Exchange.SubmitOrder(ctx, order)
	if err != nil {
		log.WithError(err).Errorf("order create error: side %s volume: %s", order.Side, order.VolumeStr)
		return
	}
}
