package grid2

import (
	"context"
	"strconv"
	"time"

	"github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/exchange/retry"
	"github.com/c9s/bbgo/pkg/types"
	"github.com/c9s/bbgo/pkg/util"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"go.uber.org/multierr"
)

type SyncActiveOrdersOpts struct {
	logger            *logrus.Entry
	metricsLabels     prometheus.Labels
	activeOrderBook   *bbgo.ActiveOrderBook
	orderQueryService types.ExchangeOrderQueryService
	exchange          types.Exchange
}

func (s *Strategy) recoverActiveOrdersPeriodically(ctx context.Context) {
	// every time we activeOrdersRecoverCh receive signal, do active orders recover
	s.activeOrdersRecoverCh = make(chan struct{}, 1)

	// make ticker's interval random in 25 min ~ 35 min
	interval := util.MillisecondsJitter(25*time.Minute, 10*60*1000)
	s.logger.Infof("[ActiveOrderRecover] interval: %s", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	opts := SyncActiveOrdersOpts{
		logger:            s.logger,
		metricsLabels:     s.newPrometheusLabels(),
		activeOrderBook:   s.orderExecutor.ActiveMakerOrders(),
		orderQueryService: s.orderQueryService,
		exchange:          s.session.Exchange,
	}

	for {
		select {

		case <-ctx.Done():
			return

		case <-ticker.C:
			if err := syncActiveOrders(ctx, opts); err != nil {
				log.WithError(err).Errorf("unable to sync active orders")
			}

		case <-s.activeOrdersRecoverCh:
			if err := syncActiveOrders(ctx, opts); err != nil {
				log.WithError(err).Errorf("unable to sync active orders")
			}

		}
	}
}

func syncActiveOrders(ctx context.Context, opts SyncActiveOrdersOpts) error {
	opts.logger.Infof("[ActiveOrderRecover] syncActiveOrders")

	notAddNonExistingOpenOrdersAfter := time.Now().Add(-5 * time.Minute)

	openOrders, err := retry.QueryOpenOrdersUntilSuccessful(ctx, opts.exchange, opts.activeOrderBook.Symbol)
	if err != nil {
		opts.logger.WithError(err).Error("[ActiveOrderRecover] failed to query open orders, skip this time")
		return errors.Wrapf(err, "[ActiveOrderRecover] failed to query open orders, skip this time")
	}

	metricsNumOfOpenOrders.With(opts.metricsLabels).Set(float64(len(openOrders)))

	activeOrders := opts.activeOrderBook.Orders()

	openOrdersMap := make(map[uint64]types.Order)
	for _, openOrder := range openOrders {
		openOrdersMap[openOrder.OrderID] = openOrder
	}

	var errs error
	// update active orders not in open orders
	for _, activeOrder := range activeOrders {
		if _, exist := openOrdersMap[activeOrder.OrderID]; exist {
			// no need to sync active order already in active orderbook, because we only need to know if it filled or not.
			delete(openOrdersMap, activeOrder.OrderID)
		} else {
			opts.logger.Infof("found active order #%d is not in the open orders, updating...", activeOrder.OrderID)

			if err := syncActiveOrder(ctx, opts.activeOrderBook, opts.orderQueryService, activeOrder.OrderID); err != nil {
				opts.logger.WithError(err).Errorf("[ActiveOrderRecover] unable to query order #%d", activeOrder.OrderID)
				errs = multierr.Append(errs, err)
				continue
			}
		}
	}

	// update open orders not in active orders
	for _, openOrder := range openOrdersMap {
		// we don't add open orders into active orderbook if updated in 5 min
		if openOrder.UpdateTime.After(notAddNonExistingOpenOrdersAfter) {
			continue
		}

		opts.activeOrderBook.Add(openOrder)
		// opts.activeOrderBook.Update(openOrder)
	}

	return errs
}

func syncActiveOrder(ctx context.Context, activeOrderBook *bbgo.ActiveOrderBook, orderQueryService types.ExchangeOrderQueryService, orderID uint64) error {
	updatedOrder, err := retry.QueryOrderUntilSuccessful(ctx, orderQueryService, types.OrderQuery{
		Symbol:  activeOrderBook.Symbol,
		OrderID: strconv.FormatUint(orderID, 10),
	})

	if err != nil {
		return err
	}

	activeOrderBook.Update(*updatedOrder)

	return nil
}
