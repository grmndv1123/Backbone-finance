package binance

import (
	"github.com/adshao/go-binance/v2"

	"github.com/c9s/bbgo/pkg/exchange/binance/binanceapi"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
)

func toGlobalLoan(record binanceapi.MarginLoanRecord) types.MarginLoanRecord {
	return types.MarginLoanRecord{
		TransactionID:  uint64(record.TxId),
		Asset:          record.Asset,
		Principle:      record.Principal,
		Time:           types.Time(record.Timestamp),
		IsolatedSymbol: record.IsolatedSymbol,
	}
}

func toGlobalRepay(record binanceapi.MarginRepayRecord) types.MarginRepayRecord {
	return types.MarginRepayRecord{
		TransactionID:  record.TxId,
		Asset:          record.Asset,
		Principle:      record.Principal,
		Time:           types.Time(record.Timestamp),
		IsolatedSymbol: record.IsolatedSymbol,
	}
}

func toGlobalInterest(record binanceapi.MarginInterest) types.MarginInterest {
	return types.MarginInterest{
		Asset:          record.Asset,
		Principle:      record.Principal,
		Interest:       record.Interest,
		InterestRate:   record.InterestRate,
		IsolatedSymbol: record.IsolatedSymbol,
		Time:           types.Time(record.InterestAccuredTime),
	}

}

func toGlobalIsolatedUserAsset(userAsset binance.IsolatedUserAsset) types.IsolatedUserAsset {
	return types.IsolatedUserAsset{
		Asset:         userAsset.Asset,
		Borrowed:      fixedpoint.MustNewFromString(userAsset.Borrowed),
		Free:          fixedpoint.MustNewFromString(userAsset.Free),
		Interest:      fixedpoint.MustNewFromString(userAsset.Interest),
		Locked:        fixedpoint.MustNewFromString(userAsset.Locked),
		NetAsset:      fixedpoint.MustNewFromString(userAsset.NetAsset),
		NetAssetOfBtc: fixedpoint.MustNewFromString(userAsset.NetAssetOfBtc),
		BorrowEnabled: userAsset.BorrowEnabled,
		RepayEnabled:  userAsset.RepayEnabled,
		TotalAsset:    fixedpoint.MustNewFromString(userAsset.TotalAsset),
	}
}

func toGlobalIsolatedMarginAsset(asset binance.IsolatedMarginAsset) types.IsolatedMarginAsset {
	return types.IsolatedMarginAsset{
		Symbol:            asset.Symbol,
		QuoteAsset:        toGlobalIsolatedUserAsset(asset.QuoteAsset),
		BaseAsset:         toGlobalIsolatedUserAsset(asset.BaseAsset),
		IsolatedCreated:   asset.IsolatedCreated,
		MarginLevel:       fixedpoint.MustNewFromString(asset.MarginLevel),
		MarginLevelStatus: asset.MarginLevelStatus,
		MarginRatio:       fixedpoint.MustNewFromString(asset.MarginRatio),
		IndexPrice:        fixedpoint.MustNewFromString(asset.IndexPrice),
		LiquidatePrice:    fixedpoint.MustNewFromString(asset.LiquidatePrice),
		LiquidateRate:     fixedpoint.MustNewFromString(asset.LiquidateRate),
		TradeEnabled:      false,
	}
}

func toGlobalIsolatedMarginAssets(assets []binance.IsolatedMarginAsset) (retAssets types.IsolatedMarginAssetMap) {
	retMarginAssets := make(types.IsolatedMarginAssetMap)
	for _, marginAsset := range assets {
		retMarginAssets[marginAsset.Symbol] = toGlobalIsolatedMarginAsset(marginAsset)
	}

	return retMarginAssets
}

func toGlobalMarginUserAssets(assets []binance.UserAsset) types.MarginAssetMap {
	retMarginAssets := make(types.MarginAssetMap)
	for _, marginAsset := range assets {
		retMarginAssets[marginAsset.Asset] = types.MarginUserAsset{
			Asset:    marginAsset.Asset,
			Borrowed: fixedpoint.MustNewFromString(marginAsset.Borrowed),
			Free:     fixedpoint.MustNewFromString(marginAsset.Free),
			Interest: fixedpoint.MustNewFromString(marginAsset.Interest),
			Locked:   fixedpoint.MustNewFromString(marginAsset.Locked),
			NetAsset: fixedpoint.MustNewFromString(marginAsset.NetAsset),
		}
	}

	return retMarginAssets
}

func toGlobalMarginAccountInfo(account *binance.MarginAccount) *types.MarginAccountInfo {
	return &types.MarginAccountInfo{
		BorrowEnabled:       account.BorrowEnabled,
		MarginLevel:         fixedpoint.MustNewFromString(account.MarginLevel),
		TotalAssetOfBTC:     fixedpoint.MustNewFromString(account.TotalAssetOfBTC),
		TotalLiabilityOfBTC: fixedpoint.MustNewFromString(account.TotalLiabilityOfBTC),
		TotalNetAssetOfBTC:  fixedpoint.MustNewFromString(account.TotalNetAssetOfBTC),
		TradeEnabled:        account.TradeEnabled,
		TransferEnabled:     account.TransferEnabled,
		Assets:              toGlobalMarginUserAssets(account.UserAssets),
	}
}

func toGlobalIsolatedMarginAccountInfo(account *binance.IsolatedMarginAccount) *types.IsolatedMarginAccountInfo {
	return &types.IsolatedMarginAccountInfo{
		TotalAssetOfBTC:     fixedpoint.MustNewFromString(account.TotalAssetOfBTC),
		TotalLiabilityOfBTC: fixedpoint.MustNewFromString(account.TotalLiabilityOfBTC),
		TotalNetAssetOfBTC:  fixedpoint.MustNewFromString(account.TotalNetAssetOfBTC),
		Assets:              toGlobalIsolatedMarginAssets(account.Assets),
	}
}
