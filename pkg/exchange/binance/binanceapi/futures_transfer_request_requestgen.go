// Code generated by "requestgen -method POST -url /sapi/v1/futures/transfer -type FuturesTransferRequest -responseType .FuturesTransferResponse"; DO NOT EDIT.

package binanceapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
)

func (f *FuturesTransferRequest) Asset(asset string) *FuturesTransferRequest {
	f.asset = asset
	return f
}

func (f *FuturesTransferRequest) Amount(amount string) *FuturesTransferRequest {
	f.amount = amount
	return f
}

func (f *FuturesTransferRequest) TransferType(transferType FuturesTransferType) *FuturesTransferRequest {
	f.transferType = transferType
	return f
}

// GetQueryParameters builds and checks the query parameters and returns url.Values
func (f *FuturesTransferRequest) GetQueryParameters() (url.Values, error) {
	var params = map[string]interface{}{}

	query := url.Values{}
	for _k, _v := range params {
		query.Add(_k, fmt.Sprintf("%v", _v))
	}

	return query, nil
}

// GetParameters builds and checks the parameters and return the result in a map object
func (f *FuturesTransferRequest) GetParameters() (map[string]interface{}, error) {
	var params = map[string]interface{}{}
	// check asset field -> json key asset
	asset := f.asset

	// assign parameter of asset
	params["asset"] = asset
	// check amount field -> json key amount
	amount := f.amount

	// assign parameter of amount
	params["amount"] = amount
	// check transferType field -> json key type
	transferType := f.transferType

	// TEMPLATE check-valid-values
	switch transferType {
	case FuturesTransferSpotToUsdtFutures, FuturesTransferUsdtFuturesToSpot, FuturesTransferSpotToCoinFutures, FuturesTransferCoinFuturesToSpot:
		params["type"] = transferType

	default:
		return nil, fmt.Errorf("type value %v is invalid", transferType)

	}
	// END TEMPLATE check-valid-values

	// assign parameter of transferType
	params["type"] = transferType

	return params, nil
}

// GetParametersQuery converts the parameters from GetParameters into the url.Values format
func (f *FuturesTransferRequest) GetParametersQuery() (url.Values, error) {
	query := url.Values{}

	params, err := f.GetParameters()
	if err != nil {
		return query, err
	}

	for _k, _v := range params {
		if f.isVarSlice(_v) {
			f.iterateSlice(_v, func(it interface{}) {
				query.Add(_k+"[]", fmt.Sprintf("%v", it))
			})
		} else {
			query.Add(_k, fmt.Sprintf("%v", _v))
		}
	}

	return query, nil
}

// GetParametersJSON converts the parameters from GetParameters into the JSON format
func (f *FuturesTransferRequest) GetParametersJSON() ([]byte, error) {
	params, err := f.GetParameters()
	if err != nil {
		return nil, err
	}

	return json.Marshal(params)
}

// GetSlugParameters builds and checks the slug parameters and return the result in a map object
func (f *FuturesTransferRequest) GetSlugParameters() (map[string]interface{}, error) {
	var params = map[string]interface{}{}

	return params, nil
}

func (f *FuturesTransferRequest) applySlugsToUrl(url string, slugs map[string]string) string {
	for _k, _v := range slugs {
		needleRE := regexp.MustCompile(":" + _k + "\\b")
		url = needleRE.ReplaceAllString(url, _v)
	}

	return url
}

func (f *FuturesTransferRequest) iterateSlice(slice interface{}, _f func(it interface{})) {
	sliceValue := reflect.ValueOf(slice)
	for _i := 0; _i < sliceValue.Len(); _i++ {
		it := sliceValue.Index(_i).Interface()
		_f(it)
	}
}

func (f *FuturesTransferRequest) isVarSlice(_v interface{}) bool {
	rt := reflect.TypeOf(_v)
	switch rt.Kind() {
	case reflect.Slice:
		return true
	}
	return false
}

func (f *FuturesTransferRequest) GetSlugsMap() (map[string]string, error) {
	slugs := map[string]string{}
	params, err := f.GetSlugParameters()
	if err != nil {
		return slugs, nil
	}

	for _k, _v := range params {
		slugs[_k] = fmt.Sprintf("%v", _v)
	}

	return slugs, nil
}

func (f *FuturesTransferRequest) Do(ctx context.Context) (*FuturesTransferResponse, error) {

	params, err := f.GetParameters()
	if err != nil {
		return nil, err
	}
	query := url.Values{}

	apiURL := "/sapi/v1/futures/transfer"

	req, err := f.client.NewAuthenticatedRequest(ctx, "POST", apiURL, query, params)
	if err != nil {
		return nil, err
	}

	response, err := f.client.SendRequest(req)
	if err != nil {
		return nil, err
	}

	var apiResponse FuturesTransferResponse
	if err := response.DecodeJSON(&apiResponse); err != nil {
		return nil, err
	}
	return &apiResponse, nil
}
