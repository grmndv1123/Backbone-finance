package indicator

import "github.com/c9s/bbgo/pkg/types"

type KLineWindowUpdater interface {
	OnKLineWindowUpdate(func(interval types.Interval, window types.KLineWindow))
}

type KLineCloseHandler interface {
	OnKLineClosed(func(k types.KLine))
}

// KLinePusher provides an interface for API user to push kline value to the indicator.
// The indicator implements its own way to calculate the value from the given kline object.
type KLinePusher interface {
	PushK(k types.KLine)
}

type KLineCalculateUpdater interface {
	CalculateAndUpdate(allKLines []types.KLine)
}
