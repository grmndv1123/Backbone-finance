package bbgo

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/codingconcepts/env"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	"github.com/c9s/bbgo/pkg/accounting/pnl"
	"github.com/c9s/bbgo/pkg/cmd/cmdutil"
	"github.com/c9s/bbgo/pkg/service"
	"github.com/c9s/bbgo/pkg/types"
	"github.com/c9s/bbgo/pkg/util"
)

var LoadedExchangeStrategies = make(map[string]SingleExchangeStrategy)
var LoadedCrossExchangeStrategies = make(map[string]CrossExchangeStrategy)

func RegisterStrategy(key string, s interface{}) {
	loaded := 0
	if d, ok := s.(SingleExchangeStrategy); ok {
		LoadedExchangeStrategies[key] = d
		loaded++
	}

	if d, ok := s.(CrossExchangeStrategy); ok {
		LoadedCrossExchangeStrategies[key] = d
		loaded++
	}

	if loaded == 0 {
		panic(fmt.Errorf("%T does not implement SingleExchangeStrategy or CrossExchangeStrategy", s))
	}
}

var emptyTime time.Time

// Environment presents the real exchange data layer
type Environment struct {
	// Notifiability here for environment is for the streaming data notification
	// note that, for back tests, we don't need notification.
	Notifiability

	PersistenceServiceFacade *service.PersistenceServiceFacade
	DatabaseService          *service.DatabaseService
	OrderService             *service.OrderService
	TradeService             *service.TradeService
	TradeSync                *service.SyncService

	// startTime is the time of start point (which is used in the backtest)
	startTime time.Time

	// syncStartTime is the time point we want to start the sync (for trades and orders)
	syncStartTime time.Time
	syncing       bool
	syncMutex     sync.Mutex

	sessions map[string]*ExchangeSession
}

func NewEnvironment() *Environment {
	return &Environment{
		// default trade scan time
		syncStartTime: time.Now().AddDate(-1, 0, 0), // defaults to sync from 1 year ago
		sessions:      make(map[string]*ExchangeSession),
		startTime:     time.Now(),

		PersistenceServiceFacade: &service.PersistenceServiceFacade{
			Memory: service.NewMemoryService(),
		},
	}
}

func (environ *Environment) Session(name string) (*ExchangeSession, bool) {
	s, ok := environ.sessions[name]
	return s, ok
}

func (environ *Environment) Sessions() map[string]*ExchangeSession {
	return environ.sessions
}

func (environ *Environment) SelectSessions(names ...string) map[string]*ExchangeSession {
	if len(names) == 0 {
		return environ.sessions
	}

	sessions := make(map[string]*ExchangeSession)
	for _, name := range names {
		if s, ok := environ.Session(name); ok {
			sessions[name] = s
		}
	}

	return sessions
}

func (environ *Environment) ConfigureDatabase(ctx context.Context) error {
	// configureDB configures the database service based on the environment variable
	if driver, ok := os.LookupEnv("DB_DRIVER"); ok {

		if dsn, ok := os.LookupEnv("DB_DSN"); ok {
			return environ.ConfigureDatabaseDriver(ctx, driver, dsn)
		}

	} else if dsn, ok := os.LookupEnv("SQLITE3_DSN"); ok {

		return environ.ConfigureDatabaseDriver(ctx, "sqlite3", dsn)

	} else if dsn, ok := os.LookupEnv("MYSQL_URL"); ok {

		return environ.ConfigureDatabaseDriver(ctx, "mysql", dsn)

	}

	return nil
}

func (environ *Environment) ConfigureDatabaseDriver(ctx context.Context, driver string, dsn string) error {
	environ.DatabaseService = service.NewDatabaseService(driver, dsn)
	err := environ.DatabaseService.Connect()
	if err != nil {
		return err
	}

	if err := environ.DatabaseService.Upgrade(ctx); err != nil {
		return err
	}

	// get the db connection pool object to create other services
	db := environ.DatabaseService.DB
	environ.OrderService = &service.OrderService{DB: db}
	environ.TradeService = &service.TradeService{DB: db}
	environ.TradeSync = &service.SyncService{
		TradeService: environ.TradeService,
		OrderService: environ.OrderService,
	}

	return nil
}

// AddExchangeSession adds the existing exchange session or pre-created exchange session
func (environ *Environment) AddExchangeSession(name string, session *ExchangeSession) *ExchangeSession {
	// update Notifiability from the environment
	session.Notifiability = environ.Notifiability

	environ.sessions[name] = session
	return session
}

// AddExchange adds the given exchange with the session name, this is the default
func (environ *Environment) AddExchange(name string, exchange types.Exchange) (session *ExchangeSession) {
	session = NewExchangeSession(name, exchange)
	return environ.AddExchangeSession(name, session)
}

func (environ *Environment) AddExchangesFromConfig(userConfig *Config) error {
	if len(userConfig.Sessions) == 0 {
		return environ.AddExchangesByViperKeys()
	}

	return environ.AddExchangesFromSessionConfig(userConfig.Sessions)
}

func (environ *Environment) AddExchangesByViperKeys() error {
	for _, n := range SupportedExchanges {
		if viper.IsSet(string(n) + "-api-key") {
			exchange, err := cmdutil.NewExchangeWithEnvVarPrefix(n, "")
			if err != nil {
				return err
			}

			environ.AddExchange(n.String(), exchange)
		}
	}

	return nil
}

func NewExchangeSessionFromConfig(name string, sessionConfig *ExchangeSession) (*ExchangeSession, error) {
	exchangeName, err := types.ValidExchangeName(sessionConfig.ExchangeName)
	if err != nil {
		return nil, err
	}

	var exchange types.Exchange

	if sessionConfig.Key != "" && sessionConfig.Secret != "" {
		exchange, err = cmdutil.NewExchangeStandard(exchangeName, sessionConfig.Key, sessionConfig.Secret)
	} else {
		exchange, err = cmdutil.NewExchangeWithEnvVarPrefix(exchangeName, sessionConfig.EnvVarPrefix)
	}

	if err != nil {
		return nil, err
	}

	// configure exchange
	if sessionConfig.Margin {
		marginExchange, ok := exchange.(types.MarginExchange)
		if !ok {
			return nil, fmt.Errorf("exchange %s does not support margin", exchangeName)
		}

		if sessionConfig.IsolatedMargin {
			marginExchange.UseIsolatedMargin(sessionConfig.IsolatedMarginSymbol)
		} else {
			marginExchange.UseMargin()
		}
	}

	session := NewExchangeSession(name, exchange)
	session.ExchangeName = sessionConfig.ExchangeName
	session.EnvVarPrefix = sessionConfig.EnvVarPrefix
	session.Key = sessionConfig.Key
	session.Secret = sessionConfig.Secret
	session.PublicOnly = sessionConfig.PublicOnly
	session.Margin = sessionConfig.Margin
	session.IsolatedMargin = sessionConfig.IsolatedMargin
	session.IsolatedMarginSymbol = sessionConfig.IsolatedMarginSymbol
	return session, nil
}

func (environ *Environment) AddExchangesFromSessionConfig(sessions map[string]*ExchangeSession) error {
	for sessionName, sessionConfig := range sessions {
		session, err := NewExchangeSessionFromConfig(sessionName, sessionConfig)
		if err != nil {
			return err
		}

		environ.AddExchangeSession(sessionName, session)
	}

	return nil
}

// Init prepares the data that will be used by the strategies
func (environ *Environment) Init(ctx context.Context) (err error) {
	for n := range environ.sessions {
		var session = environ.sessions[n]

		if err := session.Init(ctx, environ); err != nil {
			// we can skip initialized sessions
			if err != ErrSessionAlreadyInitialized {
				return err
			}
		}

		if err := session.InitSymbols(ctx, environ); err != nil {
			return err
		}
	}

	return nil
}

func (environ *Environment) ConfigurePersistence(conf *PersistenceConfig) error {
	if conf.Redis != nil {
		if err := env.Set(conf.Redis); err != nil {
			return err
		}

		environ.PersistenceServiceFacade.Redis = service.NewRedisPersistenceService(conf.Redis)
	}

	if conf.Json != nil {
		if _, err := os.Stat(conf.Json.Directory); os.IsNotExist(err) {
			if err2 := os.MkdirAll(conf.Json.Directory, 0777); err2 != nil {
				log.WithError(err2).Errorf("can not create directory: %s", conf.Json.Directory)
				return err2
			}
		}

		environ.PersistenceServiceFacade.Json = &service.JsonPersistenceService{Directory: conf.Json.Directory}
	}

	return nil
}

// configure notification rules
// for symbol-based routes, we should register the same symbol rules for each session.
// for session-based routes, we should set the fixed callbacks for each session
func (environ *Environment) ConfigureNotification(conf *NotificationConfig) error {
	// configure routing here
	if conf.SymbolChannels != nil {
		environ.SymbolChannelRouter.AddRoute(conf.SymbolChannels)
	}
	if conf.SessionChannels != nil {
		environ.SessionChannelRouter.AddRoute(conf.SessionChannels)
	}

	if conf.Routing != nil {
		// configure passive object notification routing
		switch conf.Routing.Trade {
		case "$silent": // silent, do not setup notification

		case "$session":
			defaultTradeUpdateHandler := func(trade types.Trade) {
				text := util.Render(TemplateTradeReport, trade)
				environ.Notify(text, &trade)
			}
			for name := range environ.sessions {
				session := environ.sessions[name]

				// if we can route session name to channel successfully...
				channel, ok := environ.SessionChannelRouter.Route(name)
				if ok {
					session.Stream.OnTradeUpdate(func(trade types.Trade) {
						text := util.Render(TemplateTradeReport, trade)
						environ.NotifyTo(channel, text, &trade)
					})
				} else {
					session.Stream.OnTradeUpdate(defaultTradeUpdateHandler)
				}
			}

		case "$symbol":
			// configure object routes for Trade
			environ.ObjectChannelRouter.Route(func(obj interface{}) (channel string, ok bool) {
				trade, matched := obj.(*types.Trade)
				if !matched {
					return
				}
				channel, ok = environ.SymbolChannelRouter.Route(trade.Symbol)
				return
			})

			// use same handler for each session
			handler := func(trade types.Trade) {
				text := util.Render(TemplateTradeReport, trade)
				channel, ok := environ.RouteObject(&trade)
				if ok {
					environ.NotifyTo(channel, text, &trade)
				} else {
					environ.Notify(text, &trade)
				}
			}
			for _, session := range environ.sessions {
				session.Stream.OnTradeUpdate(handler)
			}
		}

		switch conf.Routing.Order {

		case "$silent": // silent, do not setup notification

		case "$session":
			defaultOrderUpdateHandler := func(order types.Order) {
				text := util.Render(TemplateOrderReport, order)
				environ.Notify(text, &order)
			}
			for name := range environ.sessions {
				session := environ.sessions[name]

				// if we can route session name to channel successfully...
				channel, ok := environ.SessionChannelRouter.Route(name)
				if ok {
					session.Stream.OnOrderUpdate(func(order types.Order) {
						text := util.Render(TemplateOrderReport, order)
						environ.NotifyTo(channel, text, &order)
					})
				} else {
					session.Stream.OnOrderUpdate(defaultOrderUpdateHandler)
				}
			}

		case "$symbol":
			// add object route
			environ.ObjectChannelRouter.Route(func(obj interface{}) (channel string, ok bool) {
				order, matched := obj.(*types.Order)
				if !matched {
					return
				}
				channel, ok = environ.SymbolChannelRouter.Route(order.Symbol)
				return
			})

			// use same handler for each session
			handler := func(order types.Order) {
				text := util.Render(TemplateOrderReport, order)
				channel, ok := environ.RouteObject(&order)
				if ok {
					environ.NotifyTo(channel, text, &order)
				} else {
					environ.Notify(text, &order)
				}
			}
			for _, session := range environ.sessions {
				session.Stream.OnOrderUpdate(handler)
			}
		}

		switch conf.Routing.SubmitOrder {

		case "$silent": // silent, do not setup notification

		case "$symbol":
			// add object route
			environ.ObjectChannelRouter.Route(func(obj interface{}) (channel string, ok bool) {
				order, matched := obj.(*types.SubmitOrder)
				if !matched {
					return
				}

				channel, ok = environ.SymbolChannelRouter.Route(order.Symbol)
				return
			})

		}

		// currently not used
		switch conf.Routing.PnL {
		case "$symbol":
			environ.ObjectChannelRouter.Route(func(obj interface{}) (channel string, ok bool) {
				report, matched := obj.(*pnl.AverageCostPnlReport)
				if !matched {
					return
				}
				channel, ok = environ.SymbolChannelRouter.Route(report.Symbol)
				return
			})
		}

	}
	return nil
}

func (environ *Environment) SetStartTime(t time.Time) *Environment {
	environ.startTime = t
	return environ
}

// SetSyncStartTime overrides the default trade scan time (-7 days)
func (environ *Environment) SetSyncStartTime(t time.Time) *Environment {
	environ.syncStartTime = t
	return environ
}

func (environ *Environment) Connect(ctx context.Context) error {
	for n := range environ.sessions {
		// avoid using the placeholder variable for the session because we use that in the callbacks
		var session = environ.sessions[n]
		var logger = log.WithField("session", n)

		if len(session.Subscriptions) == 0 {
			logger.Warnf("exchange session %s has no subscriptions", session.Name)
		} else {
			// add the subscribe requests to the stream
			for _, s := range session.Subscriptions {
				logger.Infof("subscribing %s %s %v", s.Symbol, s.Channel, s.Options)
				session.Stream.Subscribe(s.Channel, s.Symbol, s.Options)
			}
		}

		logger.Infof("connecting session %s...", session.Name)
		if err := session.Stream.Connect(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (environ *Environment) IsSyncing() bool {
	environ.syncMutex.Lock()
	defer environ.syncMutex.Unlock()

	return environ.syncing
}

// Sync syncs all registered exchange sessions
func (environ *Environment) Sync(ctx context.Context) error {
	environ.syncMutex.Lock()
	defer environ.syncMutex.Unlock()
	environ.syncing = true

	defer func() {
		environ.syncing = false
	}()

	for _, session := range environ.sessions {
		if err := environ.syncSession(ctx, session); err != nil {
			return err
		}
	}

	return nil
}

func (environ *Environment) SyncSession(ctx context.Context, session *ExchangeSession, defaultSymbols ...string) error {
	environ.syncMutex.Lock()
	defer environ.syncMutex.Unlock()

	environ.syncing = true
	defer func() {
		environ.syncing = false
	}()

	return environ.syncSession(ctx, session, defaultSymbols...)
}

func (environ *Environment) syncSession(ctx context.Context, session *ExchangeSession, defaultSymbols ...string) error {
	symbols, err := getSessionSymbols(session, defaultSymbols...)
	if err != nil {
		return err
	}

	return environ.TradeSync.SyncSessionSymbols(ctx, session.Exchange, environ.syncStartTime, symbols...)
}

func getSessionSymbols(session *ExchangeSession, defaultSymbols ...string) ([]string, error) {
	if session.IsolatedMargin {
		return []string{session.IsolatedMarginSymbol}, nil
	}

	if len(defaultSymbols) > 0 {
		return defaultSymbols, nil
	}

	return session.FindPossibleSymbols()
}
