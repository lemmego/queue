package queue

import (
	"context"
	"errors"

	"github.com/lemmego/tasker"
)

var ErrNotInitialized = errors.New("queue: provider is not initialized")

func manager() (*tasker.Manager, error) {
	mgr := tasker.Global()
	if mgr == nil {
		return nil, ErrNotInitialized
	}
	return mgr, nil
}

func Dispatch(ctx context.Context, job tasker.Job, opts ...tasker.DispatchOpt) (*tasker.JobRow, error) {
	mgr, err := manager()
	if err != nil {
		return nil, err
	}
	return mgr.Dispatch(ctx, job, opts...)
}

func DispatchBatch(ctx context.Context, jobs []tasker.Job, opts ...tasker.DispatchOpt) ([]*tasker.JobRow, error) {
	mgr, err := manager()
	if err != nil {
		return nil, err
	}
	return mgr.DispatchBatch(ctx, jobs, opts...)
}

func Chain(ctx context.Context, jobs []tasker.Job, opts ...tasker.DispatchOpt) ([]*tasker.JobRow, error) {
	mgr, err := manager()
	if err != nil {
		return nil, err
	}
	return mgr.Chain(ctx, jobs, opts...)
}

func Retry(ctx context.Context, id tasker.JobID) (*tasker.JobRow, error) {
	mgr, err := manager()
	if err != nil {
		return nil, err
	}
	return mgr.Retry(ctx, id)
}

func Cancel(ctx context.Context, id tasker.JobID) (*tasker.JobRow, error) {
	mgr, err := manager()
	if err != nil {
		return nil, err
	}
	return mgr.Cancel(ctx, id)
}

func GetJob(ctx context.Context, id tasker.JobID) (*tasker.JobRow, error) {
	mgr, err := manager()
	if err != nil {
		return nil, err
	}
	return mgr.GetJob(ctx, id)
}

func RegisterJob(name string, factory tasker.JobFactory) {
	tasker.RegisterJob(name, factory)
}

func AddGlobalMiddleware(mw tasker.JobMiddleware) {
	tasker.AddGlobalMiddleware(mw)
}
