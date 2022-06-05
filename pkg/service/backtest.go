package service

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/c9s/bbgo/pkg/exchange/batch"
	"github.com/c9s/bbgo/pkg/types"
)

type BacktestService struct {
	DB *sqlx.DB
}

func (s *BacktestService) SyncKLineByInterval(ctx context.Context, exchange types.Exchange, symbol string, interval types.Interval, startTime, endTime time.Time) error {
	log.Infof("synchronizing %s klines with interval %s: %s <=> %s", exchange.Name(), interval, startTime, endTime)

	// TODO: use isFutures here
	_, _, isIsolated, isolatedSymbol := getExchangeAttributes(exchange)
	// override symbol if isolatedSymbol is not empty
	if isIsolated && len(isolatedSymbol) > 0 {
		symbol = isolatedSymbol
	}

	tasks := []SyncTask{
		{
			Type:   types.KLine{},
			Select: SelectLastKLines(exchange.Name(), symbol, interval, startTime, endTime, 100),
			Time: func(obj interface{}) time.Time {
				return obj.(types.KLine).StartTime.Time().UTC()
			},
			ID: func(obj interface{}) string {
				kline := obj.(types.KLine)
				return kline.Symbol + kline.Interval.String() + strconv.FormatInt(kline.StartTime.UnixMilli(), 10)
			},
			BatchQuery: func(ctx context.Context, startTime, endTime time.Time) (interface{}, chan error) {
				q := &batch.KLineBatchQuery{Exchange: exchange}
				return q.Query(ctx, symbol, interval, startTime, endTime)
			},
			Insert: func(obj interface{}) error {
				kline := obj.(types.KLine)
				return s.Insert(kline)
			},
		},
	}

	for _, sel := range tasks {
		if err := sel.execute(ctx, s.DB, startTime, endTime); err != nil {
			return err
		}
	}

	return nil
}

func (s *BacktestService) Verify(symbols []string, startTime time.Time, endTime time.Time, sourceExchange types.Exchange, verboseCnt int) error {
	var corruptCnt = 0
	for _, symbol := range symbols {
		for interval := range types.SupportedIntervals {
			log.Infof("verifying %s %s backtesting data...", symbol, interval)

			timeRanges, err := s.FindMissingTimeRanges(context.Background(), sourceExchange, symbol, interval, startTime, endTime)
			if err != nil {
				return err
			}

			if len(timeRanges) == 0 {
				continue
			}

			log.Warnf("found missing time ranges:")
			corruptCnt += len(timeRanges)
			for _, timeRange := range timeRanges {
				log.Warnf("symbol %s interval: %s %v", symbol, interval, timeRange)
			}
		}
	}

	log.Infof("backtest verification completed")
	if corruptCnt > 0 {
		log.Errorf("found %d corruptions", corruptCnt)
	} else {
		log.Infof("found %d corruptions", corruptCnt)
	}

	return nil
}

func (s *BacktestService) Sync(ctx context.Context, exchange types.Exchange, symbol string, interval types.Interval, startTime, endTime time.Time) error {

	return s.SyncKLineByInterval(ctx, exchange, symbol, interval, startTime, endTime)
}

func (s *BacktestService) QueryFirstKLine(ex types.ExchangeName, symbol string, interval types.Interval) (*types.KLine, error) {
	return s.QueryKLine(ex, symbol, interval, "ASC", 1)
}

// QueryKLine queries the klines from the database
func (s *BacktestService) QueryKLine(ex types.ExchangeName, symbol string, interval types.Interval, orderBy string, limit int) (*types.KLine, error) {
	log.Infof("querying last kline exchange = %s AND symbol = %s AND interval = %s", ex, symbol, interval)

	tableName := targetKlineTable(ex)
	// make the SQL syntax IDE friendly, so that it can analyze it.
	sql := fmt.Sprintf("SELECT * FROM `%s` WHERE  `symbol` = :symbol AND `interval` = :interval ORDER BY end_time "+orderBy+" LIMIT "+strconv.Itoa(limit), tableName)

	rows, err := s.DB.NamedQuery(sql, map[string]interface{}{
		"interval": interval,
		"symbol":   symbol,
	})

	if err != nil {
		return nil, errors.Wrap(err, "query kline error")
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	defer rows.Close()

	if rows.Next() {
		var kline types.KLine
		err = rows.StructScan(&kline)
		return &kline, err
	}

	return nil, rows.Err()
}

// QueryKLinesForward is used for querying klines to back-testing
func (s *BacktestService) QueryKLinesForward(exchange types.ExchangeName, symbol string, interval types.Interval, startTime time.Time, limit int) ([]types.KLine, error) {
	tableName := targetKlineTable(exchange)
	sql := "SELECT * FROM `binance_klines` WHERE `end_time` >= :start_time AND `symbol` = :symbol AND `interval` = :interval and exchange = :exchange ORDER BY end_time ASC LIMIT :limit"
	sql = strings.ReplaceAll(sql, "binance_klines", tableName)

	rows, err := s.DB.NamedQuery(sql, map[string]interface{}{
		"start_time": startTime,
		"limit":      limit,
		"symbol":     symbol,
		"interval":   interval,
		"exchange":   exchange.String(),
	})
	if err != nil {
		return nil, err
	}

	return s.scanRows(rows)
}

func (s *BacktestService) QueryKLinesBackward(exchange types.ExchangeName, symbol string, interval types.Interval, endTime time.Time, limit int) ([]types.KLine, error) {
	tableName := targetKlineTable(exchange)

	sql := "SELECT * FROM `binance_klines` WHERE `end_time` <= :end_time  and exchange = :exchange  AND `symbol` = :symbol AND `interval` = :interval ORDER BY end_time DESC LIMIT :limit"
	sql = strings.ReplaceAll(sql, "binance_klines", tableName)
	sql = "SELECT t.* FROM (" + sql + ") AS t ORDER BY t.end_time ASC"

	rows, err := s.DB.NamedQuery(sql, map[string]interface{}{
		"limit":    limit,
		"end_time": endTime,
		"symbol":   symbol,
		"interval": interval,
		"exchange": exchange.String(),
	})
	if err != nil {
		return nil, err
	}

	return s.scanRows(rows)
}

func (s *BacktestService) QueryKLinesCh(since, until time.Time, exchange types.Exchange, symbols []string, intervals []types.Interval) (chan types.KLine, chan error) {

	if len(symbols) == 0 {
		return returnError(errors.Errorf("symbols is empty when querying kline, plesae check your strategy setting. "))
	}

	tableName := targetKlineTable(exchange.Name())
	sql := "SELECT * FROM `binance_klines` WHERE `end_time` BETWEEN :since AND :until AND `symbol` IN (:symbols) AND `interval` IN (:intervals)  and exchange = :exchange  ORDER BY end_time ASC"
	sql = strings.ReplaceAll(sql, "binance_klines", tableName)

	sql, args, err := sqlx.Named(sql, map[string]interface{}{
		"since":     since,
		"until":     until,
		"symbols":   symbols,
		"intervals": types.IntervalSlice(intervals),
		"exchange":  exchange.Name().String(),
	})

	sql, args, err = sqlx.In(sql, args...)
	if err != nil {
		return returnError(err)
	}
	sql = s.DB.Rebind(sql)

	rows, err := s.DB.Queryx(sql, args...)
	if err != nil {
		return returnError(err)
	}

	return s.scanRowsCh(rows)
}

func returnError(err error) (chan types.KLine, chan error) {
	ch := make(chan types.KLine, 0)
	close(ch)
	log.WithError(err).Error("backtest query error")

	errC := make(chan error, 1)
	// avoid blocking
	go func() {
		errC <- err
		close(errC)
	}()
	return ch, errC
}

// scanRowsCh scan rows into channel
func (s *BacktestService) scanRowsCh(rows *sqlx.Rows) (chan types.KLine, chan error) {
	ch := make(chan types.KLine, 500)
	errC := make(chan error, 1)

	go func() {
		defer close(errC)
		defer close(ch)
		defer rows.Close()

		for rows.Next() {
			var kline types.KLine
			if err := rows.StructScan(&kline); err != nil {
				errC <- err
				return
			}

			ch <- kline
		}

		if err := rows.Err(); err != nil {
			errC <- err
			return
		}

	}()

	return ch, errC
}

func (s *BacktestService) scanRows(rows *sqlx.Rows) (klines []types.KLine, err error) {
	for rows.Next() {
		var kline types.KLine
		if err := rows.StructScan(&kline); err != nil {
			return nil, err
		}

		klines = append(klines, kline)
	}

	return klines, rows.Err()
}

func targetKlineTable(exchangeName types.ExchangeName) string {
	return strings.ToLower(exchangeName.String()) + "_klines"
}

var errExchangeFieldIsUnset = errors.New("kline.Exchange field should not be empty")

func (s *BacktestService) Insert(kline types.KLine) error {
	if len(kline.Exchange) == 0 {
		return errExchangeFieldIsUnset
	}

	tableName := targetKlineTable(kline.Exchange)

	sql := fmt.Sprintf("INSERT INTO `%s` (`exchange`, `start_time`, `end_time`, `symbol`, `interval`, `open`, `high`, `low`, `close`, `closed`, `volume`, `quote_volume`, `taker_buy_base_volume`, `taker_buy_quote_volume`)"+
		"VALUES (:exchange, :start_time, :end_time, :symbol, :interval, :open, :high, :low, :close, :closed, :volume, :quote_volume, :taker_buy_base_volume, :taker_buy_quote_volume)", tableName)

	_, err := s.DB.NamedExec(sql, kline)
	return err
}

func (s *BacktestService) _deleteDuplicatedKLine(k types.KLine) error {
	if len(k.Exchange) == 0 {
		return errors.New("kline.Exchange field should not be empty")
	}

	tableName := targetKlineTable(k.Exchange)
	sql := fmt.Sprintf("DELETE FROM `%s` WHERE gid = :gid  ", tableName)
	_, err := s.DB.NamedExec(sql, k)
	return err
}

type TimeRange struct {
	Start time.Time
	End   time.Time
}

// SyncPartial
// find the existing data time range (t1, t2)
// scan if there is a missing part
// create a time range slice []TimeRange
// iterate the []TimeRange slice to sync data.
func (s *BacktestService) SyncPartial(ctx context.Context, ex types.Exchange, symbol string, interval types.Interval, since, until time.Time) error {
	// truncate time point to minute
	since = since.Truncate(time.Minute)
	until = until.Truncate(time.Minute)

	t1, t2, err := s.QueryExistingDataRange(ctx, ex, symbol, interval, since, until)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	if err == sql.ErrNoRows {
		// fallback to fresh sync
		return s.Sync(ctx, ex, symbol, interval, since, until)
	}

	log.Debugf("found existing kline data, now using partial sync...")
	timeRanges, err := s.FindMissingTimeRanges(ctx, ex, symbol, interval, t1.Time(), t2.Time())
	if err != nil {
		return err
	}

	if len(timeRanges) > 0 {
		log.Infof("found missing time ranges: %v", timeRanges)
	}

	// there are few cases:
	// t1 == since && t2 == until
	// [since] ------- [t1] data [t2] ------ [until]
	if since.Before(t1.Time()) {
		// shift slice
		timeRanges = append([]TimeRange{
			{Start: since.Add(-2 * time.Second), End: t1.Time()}, // include since
		}, timeRanges...)
	}

	if t2.Time().Before(until) {
		timeRanges = append(timeRanges, TimeRange{
			Start: t2.Time(),
			End:   until.Add(2 * time.Second), // include until
		})
	}

	for _, timeRange := range timeRanges {
		err = s.SyncKLineByInterval(ctx, ex, symbol, types.Interval1h, timeRange.Start.Add(time.Second), timeRange.End.Add(-time.Second))
		if err != nil {
			return err
		}
	}

	return nil
}

// FindMissingTimeRanges returns the missing time ranges, the start/end time represents the existing data time points.
// So when sending kline query to the exchange API, we need to add one second to the start time and minus one second to the end time.
func (s *BacktestService) FindMissingTimeRanges(ctx context.Context, ex types.Exchange, symbol string, interval types.Interval, since, until time.Time) ([]TimeRange, error) {
	query := SelectKLineTimePoints(ex.Name(), symbol, interval, since, until)
	sql, args, err := query.ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := s.DB.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}

	var timeRanges []TimeRange
	var lastTime time.Time
	var intervalDuration = interval.Duration()
	for rows.Next() {
		var tt types.Time
		if err := rows.Scan(&tt); err != nil {
			return nil, err
		}

		var t = time.Time(tt)
		if lastTime != (time.Time{}) && t.Sub(lastTime) > intervalDuration {
			timeRanges = append(timeRanges, TimeRange{
				Start: lastTime,
				End:   t,
			})
		}

		lastTime = t
	}

	return timeRanges, nil
}

func (s *BacktestService) QueryExistingDataRange(ctx context.Context, ex types.Exchange, symbol string, interval types.Interval, tArgs ...time.Time) (start, end *types.Time, err error) {
	sel := SelectKLineTimeRange(ex.Name(), symbol, interval, tArgs...)
	sql, args, err := sel.ToSql()
	if err != nil {
		return nil, nil, err
	}

	var t1, t2 types.Time

	row := s.DB.QueryRowContext(ctx, sql, args...)

	if err := row.Scan(&t1, &t2); err != nil {
		return nil, nil, err
	}

	if err := row.Err(); err != nil {
		return nil, nil, err
	}

	return &t1, &t2, nil
}

func SelectKLineTimePoints(ex types.ExchangeName, symbol string, interval types.Interval, args ...time.Time) sq.SelectBuilder {
	conditions := sq.And{
		sq.Eq{"symbol": symbol},
		sq.Eq{"`interval`": interval.String()},
	}

	if len(args) == 2 {
		since := args[0]
		until := args[1]
		conditions = append(conditions, sq.Expr("`start_time` BETWEEN ? AND ?", since, until))
	}

	tableName := targetKlineTable(ex)

	return sq.Select("start_time").
		From(tableName).
		Where(conditions).
		OrderBy("start_time ASC")
}

// SelectKLineTimeRange returns the existing klines time range (since < kline.start_time < until)
func SelectKLineTimeRange(ex types.ExchangeName, symbol string, interval types.Interval, args ...time.Time) sq.SelectBuilder {
	conditions := sq.And{
		sq.Eq{"symbol": symbol},
		sq.Eq{"`interval`": interval.String()},
	}

	if len(args) == 2 {
		since := args[0]
		until := args[1]
		conditions = append(conditions, sq.Expr("`start_time` BETWEEN ? AND ?", since, until))
	}

	tableName := targetKlineTable(ex)

	return sq.Select("MIN(start_time) AS t1, MAX(start_time) AS t2").
		From(tableName).
		Where(conditions)
}

// TODO: add is_futures column since the klines data is different
func SelectLastKLines(ex types.ExchangeName, symbol string, interval types.Interval, startTime, endTime time.Time, limit uint64) sq.SelectBuilder {
	tableName := targetKlineTable(ex)
	return sq.Select("*").
		From(tableName).
		Where(sq.And{
			sq.Eq{"symbol": symbol},
			sq.Eq{"`interval`": interval.String()},
			sq.Expr("start_time BETWEEN ? AND ?", startTime, endTime),
		}).
		OrderBy("start_time DESC").
		Limit(limit)
}
