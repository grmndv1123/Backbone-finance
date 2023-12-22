package dca2

import (
	"context"
	"time"
)

type State int64

const (
	None State = iota
	WaitToOpenPosition
	PositionOpening
	OpenPositionReady
	OpenPositionOrderFilled
	OpenPositionOrdersCancelling
	OpenPositionOrdersCancelled
	TakeProfitReady
)

func (s *Strategy) initializeNextStateC() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	isInitialize := false
	if s.nextStateC == nil {
		s.logger.Info("[DCA] initializing next state channel")
		s.nextStateC = make(chan State, 1)
	} else {
		s.logger.Info("[DCA] nextStateC is already initialized")
		isInitialize = true
	}

	return isInitialize
}

func (s *Strategy) emitNextState(nextState State) {
	select {
	case s.nextStateC <- nextState:
	default:
		s.logger.Info("[DCA] nextStateC is full or not initialized")
	}
}

// runState
// WaitToOpenPosition -> after startTimeOfNextRound, place dca orders ->
// PositionOpening
// OpenPositionReady -> any dca maker order filled ->
// OpenPositionOrderFilled -> price hit the take profit ration, start cancelling ->
// OpenPositionOrdersCancelled -> place the takeProfit order ->
// TakeProfitReady -> the takeProfit order filled ->
func (s *Strategy) runState(ctx context.Context) {
	s.logger.Info("[DCA] runState")
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("[DCA] runState DONE")
			return
		case <-ticker.C:
			s.triggerNextState()
		case nextState := <-s.nextStateC:
			s.logger.Infof("[DCA] currenct state: %d, next state: %d", s.state, nextState)
			switch s.state {
			case WaitToOpenPosition:
				s.runWaitToOpenPositionState(ctx, nextState)
			case PositionOpening:
				s.runPositionOpening(ctx, nextState)
			case OpenPositionReady:
				s.runOpenPositionReady(ctx, nextState)
			case OpenPositionOrderFilled:
				s.runOpenPositionOrderFilled(ctx, nextState)
			case OpenPositionOrdersCancelling:
				s.runOpenPositionOrdersCancelling(ctx, nextState)
			case OpenPositionOrdersCancelled:
				s.runOpenPositionOrdersCancelled(ctx, nextState)
			case TakeProfitReady:
				s.runTakeProfitReady(ctx, nextState)
			}
		}
	}
}

func (s *Strategy) triggerNextState() {
	switch s.state {
	case WaitToOpenPosition:
		s.nextStateC <- PositionOpening
	case PositionOpening:
		s.nextStateC <- OpenPositionReady
	case OpenPositionReady:
		// trigger from order filled event
	case OpenPositionOrderFilled:
		// trigger from kline event
	case OpenPositionOrdersCancelling:
		s.nextStateC <- OpenPositionOrdersCancelled
	case OpenPositionOrdersCancelled:
		s.nextStateC <- TakeProfitReady
	case TakeProfitReady:
		// trigger from order filled event
	}
}

func (s *Strategy) runWaitToOpenPositionState(_ context.Context, next State) {
	if next != PositionOpening {
		return
	}

	s.logger.Info("[State] WaitToOpenPosition - check startTimeOfNextRound")
	if time.Now().Before(s.startTimeOfNextRound) {
		return
	}

	s.state = PositionOpening
	s.logger.Info("[State] WaitToOpenPosition -> PositionOpening")
}

func (s *Strategy) runPositionOpening(ctx context.Context, next State) {
	if next != OpenPositionReady {
		return
	}

	s.logger.Info("[State] PositionOpening - start placing open-position orders")
	if err := s.placeOpenPositionOrders(ctx); err != nil {
		s.logger.WithError(err).Error("failed to place dca orders, please check it.")
		return
	}
	s.state = OpenPositionReady
	s.logger.Info("[State] PositionOpening -> OpenPositionReady")
}

func (s *Strategy) runOpenPositionReady(_ context.Context, next State) {
	if next != OpenPositionOrderFilled {
		return
	}
	s.state = OpenPositionOrderFilled
	s.logger.Info("[State] OpenPositionReady -> OpenPositionOrderFilled")
}

func (s *Strategy) runOpenPositionOrderFilled(_ context.Context, next State) {
	if next != OpenPositionOrdersCancelling {
		return
	}
	s.state = OpenPositionOrdersCancelling
	s.logger.Info("[State] OpenPositionOrderFilled -> OpenPositionOrdersCancelling")

	// after open position cancelling, immediately trigger open position cancelled to cancel the other orders
	s.nextStateC <- OpenPositionOrdersCancelled
}

func (s *Strategy) runOpenPositionOrdersCancelling(ctx context.Context, next State) {
	if next != OpenPositionOrdersCancelled {
		return
	}

	s.logger.Info("[State] OpenPositionOrdersCancelling - start cancelling open-position orders")
	if err := s.cancelOpenPositionOrders(ctx); err != nil {
		s.logger.WithError(err).Error("failed to cancel maker orders")
		return
	}
	s.state = OpenPositionOrdersCancelled
	s.logger.Info("[State] OpenPositionOrdersCancelling -> OpenPositionOrdersCancelled")

	// after open position cancelled, immediately trigger take profit ready to open take-profit order
	s.nextStateC <- TakeProfitReady
}

func (s *Strategy) runOpenPositionOrdersCancelled(ctx context.Context, next State) {
	if next != TakeProfitReady {
		return
	}

	s.logger.Info("[State] OpenPositionOrdersCancelled - start placing take-profit orders")
	if err := s.placeTakeProfitOrders(ctx); err != nil {
		s.logger.WithError(err).Error("failed to open take profit orders")
		return
	}
	s.state = TakeProfitReady
	s.logger.Info("[State] OpenPositionOrdersCancelled -> TakeProfitReady")
}

func (s *Strategy) runTakeProfitReady(_ context.Context, next State) {
	if next != WaitToOpenPosition {
		return
	}

	s.logger.Info("[State] TakeProfitReady - start reseting position and calculate budget for next round")
	if s.Short {
		s.Budget = s.Budget.Add(s.Position.Base)
	} else {
		s.Budget = s.Budget.Add(s.Position.Quote)
	}

	// reset position
	s.Position.Reset()

	// set the start time of the next round
	s.startTimeOfNextRound = time.Now().Add(s.CoolDownInterval.Duration())
	s.state = WaitToOpenPosition
	s.logger.Info("[State] TakeProfitReady -> WaitToOpenPosition")
}
