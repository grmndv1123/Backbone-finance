package bbgo

import (
	"context"

	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	log "github.com/sirupsen/logrus"

	"github.com/c9s/bbgo/pkg/accounting/pnl"
	"github.com/c9s/bbgo/pkg/types"

	_ "github.com/go-sql-driver/mysql"
	flag "github.com/spf13/pflag"
)

var SupportedExchanges = []types.ExchangeName{"binance", "max"}

// PersistentFlags defines the flags for environments
func PersistentFlags(flags *flag.FlagSet) {
	flags.String("binance-api-key", "", "binance api key")
	flags.String("binance-api-secret", "", "binance api secret")
	flags.String("max-api-key", "", "max api key")
	flags.String("max-api-secret", "", "max api secret")
}

// SingleExchangeStrategy represents the single Exchange strategy
type SingleExchangeStrategy interface {
	Run(ctx context.Context, orderExecutor types.OrderExecutor, session *ExchangeSession) error
}

type CrossExchangeStrategy interface {
	Run(ctx context.Context, orderExecutionRouter types.OrderExecutionRouter, sessions map[string]*ExchangeSession) error
}

type PnLReporter interface {
	Run()
}

type baseReporter struct {
	notifier    Notifier
	cron        *cron.Cron
	environment *Environment
}

type PnLReporterManager struct {
	baseReporter

	reporters []PnLReporter
}

func NewPnLReporter(notifier Notifier) *PnLReporterManager {
	return &PnLReporterManager{
		baseReporter: baseReporter{
			notifier: notifier,
			cron:     cron.New(),
		},
	}
}

func (manager *PnLReporterManager) AverageCostBySymbols(symbols ...string) *AverageCostPnLReporter {
	reporter := &AverageCostPnLReporter{
		baseReporter: manager.baseReporter,
		Symbols:      symbols,
	}

	manager.reporters = append(manager.reporters, reporter)
	return reporter
}

type AverageCostPnLReporter struct {
	baseReporter

	Sessions []string
	Symbols  []string
}

func (reporter *AverageCostPnLReporter) Of(sessions ...string) *AverageCostPnLReporter {
	reporter.Sessions = sessions
	return reporter
}

func (reporter *AverageCostPnLReporter) When(specs ...string) *AverageCostPnLReporter {
	for _, spec := range specs {
		_, err := reporter.cron.AddJob(spec, reporter)
		if err != nil {
			panic(err)
		}
	}

	return reporter
}

func (reporter *AverageCostPnLReporter) Run() {
	for _, sessionName := range reporter.Sessions {
		session := reporter.environment.sessions[sessionName]
		calculator := &pnl.AverageCostCalculator{
			TradingFeeCurrency: session.Exchange.PlatformFeeCurrency(),
		}

		for _, symbol := range reporter.Symbols {
			report := calculator.Calculate(symbol, session.Trades[symbol], session.lastPrices[symbol])
			report.Print()
		}
	}
}

type Trader struct {
	Notifiability

	environment *Environment

	crossExchangeStrategies []CrossExchangeStrategy
	exchangeStrategies      map[string][]SingleExchangeStrategy

	// reportTimer             *time.Timer
	// ProfitAndLossCalculator *accounting.ProfitAndLossCalculator
}

func NewTrader(environ *Environment) *Trader {
	return &Trader{
		environment:        environ,
		exchangeStrategies: make(map[string][]SingleExchangeStrategy),
	}
}

// AttachStrategyOn attaches the single exchange strategy on an exchange session.
// Single exchange strategy is the default behavior.
func (trader *Trader) AttachStrategyOn(session string, strategies ...SingleExchangeStrategy) *Trader {
	if _, ok := trader.environment.sessions[session]; !ok {
		log.Panicf("session %s is not defined", session)
	}

	for _, s := range strategies {
		trader.exchangeStrategies[session] = append(trader.exchangeStrategies[session], s)
	}

	return trader
}

// AttachCrossExchangeStrategy attaches the cross exchange strategy
func (trader *Trader) AttachCrossExchangeStrategy(strategy CrossExchangeStrategy) *Trader {
	trader.crossExchangeStrategies = append(trader.crossExchangeStrategies, strategy)

	return trader
}

func (trader *Trader) Run(ctx context.Context) error {
	if err := trader.environment.Init(ctx); err != nil {
		return err
	}

	// load and run session strategies
	for sessionName, strategies := range trader.exchangeStrategies {
		session := trader.environment.sessions[sessionName]
		// we can move this to the exchange session,
		// that way we can mount the notification on the exchange with DSL
		orderExecutor := &ExchangeOrderExecutor{
			Notifiability: trader.Notifiability,
			Session:       session,
		}

		for _, strategy := range strategies {
			err := strategy.Run(ctx, orderExecutor, session)
			if err != nil {
				return err
			}
		}
	}

	router := &ExchangeOrderExecutionRouter{
		// copy the parent notifiers
		Notifiability: trader.Notifiability,
		sessions:      trader.environment.sessions,
	}

	for _, strategy := range trader.crossExchangeStrategies {
		if err := strategy.Run(ctx, router, trader.environment.sessions); err != nil {
			return err
		}
	}

	return trader.environment.Connect(ctx)
}

/*
func (trader *OrderExecutor) RunStrategyWithHotReload(ctx context.Context, strategy SingleExchangeStrategy, configFile string) (chan struct{}, error) {
	var done = make(chan struct{})
	var configWatcherDone = make(chan struct{})

	log.Infof("watching config file: %v", configFile)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	defer watcher.Close()

	if err := watcher.Add(configFile); err != nil {
		return nil, err
	}

	go func() {
		strategyContext, strategyCancel := context.WithCancel(ctx)
		defer strategyCancel()
		defer close(done)

		traderDone, err := trader.RunStrategy(strategyContext, strategy)
		if err != nil {
			return
		}

		var configReloadTimer *time.Timer = nil
		defer close(configWatcherDone)

		for {
			select {

			case <-ctx.Done():
				return

			case <-traderDone:
				log.Infof("reloading config file %s", configFile)
				if err := config.LoadConfigFile(configFile, strategy); err != nil {
					log.WithError(err).Error("error load config file")
				}

				trader.NotifyTo("config reloaded, restarting trader")

				traderDone, err = trader.RunStrategy(strategyContext, strategy)
				if err != nil {
					log.WithError(err).Error("[trader] error:", err)
					return
				}

			case event := <-watcher.Events:
				log.Infof("[fsnotify] event: %+v", event)

				if event.Op&fsnotify.Write == fsnotify.Write {
					log.Info("[fsnotify] modified file:", event.Name)
				}

				if configReloadTimer != nil {
					configReloadTimer.Stop()
				}

				configReloadTimer = time.AfterFunc(3*time.Second, func() {
					strategyCancel()
				})

			case err := <-watcher.Errors:
				log.WithError(err).Error("[fsnotify] error:", err)
				return

			}
		}
	}()

	return done, nil
}
*/

/*
func (trader *OrderExecutor) RunStrategy(ctx context.Context, strategy SingleExchangeStrategy) (chan struct{}, error) {
	trader.reportTimer = time.AfterFunc(1*time.Second, func() {
		trader.reportPnL()
	})

	stream.OnTrade(func(trade *types.Trade) {
		trader.NotifyTrade(trade)
		trader.ProfitAndLossCalculator.AddTrade(*trade)
		_, err := trader.Context.StockManager.AddTrades([]types.Trade{*trade})
		if err != nil {
			log.WithError(err).Error("stock manager load trades error")
		}

		if trader.reportTimer != nil {
			trader.reportTimer.Stop()
		}

		trader.reportTimer = time.AfterFunc(1*time.Minute, func() {
			trader.reportPnL()
		})
	})
}
*/

/*
func (trader *Trader) reportPnL() {
	report := trader.ProfitAndLossCalculator.Calculate()
	report.Print()
	trader.NotifyPnL(report)
}
*/

func (trader *Trader) ReportPnL(notifier Notifier) *PnLReporterManager {
	return NewPnLReporter(notifier)
}

func (trader *Trader) SubmitOrder(ctx context.Context, order types.SubmitOrder) {
	trader.Notify(":memo: Submitting %s %s %s order with quantity: %s", order.Symbol, order.Type, order.Side, order.QuantityString, order)

	orderProcessor := &OrderProcessor{
		MinQuoteBalance: 0,
		MaxAssetBalance: 0,
		MinAssetBalance: 0,
		MinProfitSpread: 0,
		MaxOrderAmount:  0,
		// FIXME:
		// Exchange:        trader.Exchange,
		Trader: trader,
	}

	err := orderProcessor.Submit(ctx, order)

	if err != nil {
		log.WithError(err).Errorf("order create error: side %s quantity: %s", order.Side, order.QuantityString)
		return
	}
}

type ExchangeOrderExecutionRouter struct {
	Notifiability

	sessions map[string]*ExchangeSession
}

func (e *ExchangeOrderExecutionRouter) SubmitOrdersTo(ctx context.Context, session string, orders ...types.SubmitOrder) error {
	es, ok := e.sessions[session]
	if !ok {
		return errors.Errorf("exchange session %s not found", session)
	}

	for _, order := range orders {
		market, ok := es.Market(order.Symbol)
		if !ok {
			return errors.Errorf("market is not defined: %s", order.Symbol)
		}

		order.PriceString = market.FormatPrice(order.Price)
		order.QuantityString = market.FormatVolume(order.Quantity)
		e.Notify(":memo: Submitting order to %s %s %s %s with quantity: %s", session, order.Symbol, order.Type, order.Side, order.QuantityString, order)

		if err := es.Exchange.SubmitOrders(ctx, order); err != nil {
			return err
		}
	}

	return nil
}

// ExchangeOrderExecutor is an order executor wrapper for single exchange instance.
type ExchangeOrderExecutor struct {
	Notifiability

	Session *ExchangeSession
}

func (e *ExchangeOrderExecutor) SubmitOrders(ctx context.Context, orders ...types.SubmitOrder) error {
	for _, order := range orders {
		market, ok := e.Session.Market(order.Symbol)
		if !ok {
			return errors.Errorf("market is not defined: %s", order.Symbol)
		}

		order.Market = market
		order.PriceString = market.FormatPrice(order.Price)
		order.QuantityString = market.FormatVolume(order.Quantity)

		e.Notify(":memo: Submitting %s %s %s order with quantity: %s", order.Symbol, order.Type, order.Side, order.QuantityString, order)

		return e.Session.Exchange.SubmitOrders(ctx, order)
	}

	return nil
}
