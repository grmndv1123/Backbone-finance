// Code generated by "callbackgen -type StandardPrivateStream -interface"; DO NOT EDIT.

package types

import (
	"reflect"
)

func (stream *StandardPrivateStream) OnTrade(cb func(trade *Trade)) {
	stream.tradeCallbacks = append(stream.tradeCallbacks, cb)
}

func (stream *StandardPrivateStream) EmitTrade(trade *Trade) {
	for _, cb := range stream.tradeCallbacks {
		cb(trade)
	}
}

func (stream *StandardPrivateStream) RemoveOnTrade(needle func(trade *Trade)) (found bool) {

	var newcallbacks []func(trade *Trade)
	var fp = reflect.ValueOf(needle).Pointer()
	for _, cb := range stream.tradeCallbacks {
		if fp == reflect.ValueOf(cb).Pointer() {
			found = true
		} else {
			newcallbacks = append(newcallbacks, cb)
		}
	}

	if found {
		stream.tradeCallbacks = newcallbacks
	}

	return found
}

func (stream *StandardPrivateStream) OnBalanceSnapshot(cb func(balanceSnapshot map[string]Balance)) {
	stream.balanceSnapshotCallbacks = append(stream.balanceSnapshotCallbacks, cb)
}

func (stream *StandardPrivateStream) EmitBalanceSnapshot(balanceSnapshot map[string]Balance) {
	for _, cb := range stream.balanceSnapshotCallbacks {
		cb(balanceSnapshot)
	}
}

func (stream *StandardPrivateStream) RemoveOnBalanceSnapshot(needle func(balanceSnapshot map[string]Balance)) (found bool) {

	var newcallbacks []func(balanceSnapshot map[string]Balance)
	var fp = reflect.ValueOf(needle).Pointer()
	for _, cb := range stream.balanceSnapshotCallbacks {
		if fp == reflect.ValueOf(cb).Pointer() {
			found = true
		} else {
			newcallbacks = append(newcallbacks, cb)
		}
	}

	if found {
		stream.balanceSnapshotCallbacks = newcallbacks
	}

	return found
}

func (stream *StandardPrivateStream) OnKLineClosed(cb func(kline *KLine)) {
	stream.kLineClosedCallbacks = append(stream.kLineClosedCallbacks, cb)
}

func (stream *StandardPrivateStream) EmitKLineClosed(kline *KLine) {
	for _, cb := range stream.kLineClosedCallbacks {
		cb(kline)
	}
}

func (stream *StandardPrivateStream) RemoveOnKLineClosed(needle func(kline *KLine)) (found bool) {

	var newcallbacks []func(kline *KLine)
	var fp = reflect.ValueOf(needle).Pointer()
	for _, cb := range stream.kLineClosedCallbacks {
		if fp == reflect.ValueOf(cb).Pointer() {
			found = true
		} else {
			newcallbacks = append(newcallbacks, cb)
		}
	}

	if found {
		stream.kLineClosedCallbacks = newcallbacks
	}

	return found
}

type StandardPrivateStreamEventHub interface {
	OnTrade(cb func(trade *Trade))

	OnBalanceSnapshot(cb func(balanceSnapshot map[string]Balance))

	OnKLineClosed(cb func(kline *KLine))
}
