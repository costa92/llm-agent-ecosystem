package service

import "context"

type RecallObservation struct {
	ConsistencyLevel string
	CacheLevel       string
	StaleServed      bool
}

type RecallObserver interface {
	ObserveRecall(ctx context.Context, obs RecallObservation)
}

type nopRecallObserver struct{}

func (nopRecallObserver) ObserveRecall(context.Context, RecallObservation) {}

func resolveRecallObserver(observer RecallObserver) RecallObserver {
	if observer != nil {
		return observer
	}
	return nopRecallObserver{}
}
