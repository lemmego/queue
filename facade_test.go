package queue

import (
	"context"
	"errors"
	"testing"

	"github.com/lemmego/tasker"
)

func TestFacadeBeforeInitialization(t *testing.T) {
	previous := tasker.Global()
	tasker.SetGlobal(nil)
	t.Cleanup(func() { tasker.SetGlobal(previous) })

	if _, err := GetJob(context.Background(), tasker.JobID(1)); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("GetJob error = %v", err)
	}
	if _, err := DispatchBatch(context.Background(), nil); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("DispatchBatch error = %v", err)
	}
}
