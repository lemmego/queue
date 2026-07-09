package queue

import (
	"context"

	"github.com/lemmego/tasker"
)

func Dispatch(ctx context.Context, job tasker.Job, opts ...tasker.DispatchOpt) (*tasker.JobRow, error) {
	return tasker.Global().Dispatch(ctx, job, opts...)
}

func DispatchBatch(ctx context.Context, jobs []tasker.Job, opts ...tasker.DispatchOpt) ([]*tasker.JobRow, error) {
	return tasker.Global().DispatchBatch(ctx, jobs, opts...)
}

func Chain(ctx context.Context, jobs []tasker.Job, opts ...tasker.DispatchOpt) ([]*tasker.JobRow, error) {
	return tasker.Global().Chain(ctx, jobs, opts...)
}

func Retry(ctx context.Context, id tasker.JobID) (*tasker.JobRow, error) {
	return tasker.Global().Retry(ctx, id)
}

func Cancel(ctx context.Context, id tasker.JobID) (*tasker.JobRow, error) {
	return tasker.Global().Cancel(ctx, id)
}

func GetJob(ctx context.Context, id tasker.JobID) (*tasker.JobRow, error) {
	return tasker.Global().GetJob(ctx, id)
}

func RegisterJob(name string, factory tasker.JobFactory) {
	tasker.RegisterJob(name, factory)
}

func AddGlobalMiddleware(mw tasker.JobMiddleware) {
	tasker.AddGlobalMiddleware(mw)
}
