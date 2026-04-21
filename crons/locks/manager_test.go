package locks

import (
	"testing"

	_ "github.com/lenchik/logmonitor/internal/testsupport"
	"github.com/ozontech/allure-go/pkg/framework/provider"
	"github.com/ozontech/allure-go/pkg/framework/runner"
)

func TestManagerTryLockIsNonBlockingAndReleasesKeys(t *testing.T) {
	runner.Run(t, "lock manager guards keys", func(t provider.T) {
		t.Epic("Crons")
		t.Feature("Lock manager")
		t.Story("Per-server isolation")
		t.Title("TryLock returns false for busy keys and true after unlock")

		manager := NewManager()
		var unlock func()

		t.WithNewStep("Acquire key for the first time", func(step provider.StepCtx) {
			var ok bool
			unlock, ok = manager.TryLock("server:srv-1")
			step.Require().True(ok)
			step.Require().NotNil(unlock)
		})

		t.WithNewStep("Reject second acquisition while key is busy", func(step provider.StepCtx) {
			secondUnlock, ok := manager.TryLock("server:srv-1")
			step.Require().False(ok)
			step.Require().Nil(secondUnlock)
		})

		t.WithNewStep("Allow acquisition after unlock", func(step provider.StepCtx) {
			unlock()
			secondUnlock, ok := manager.TryLock("server:srv-1")
			step.Require().True(ok)
			step.Require().NotNil(secondUnlock)
			secondUnlock()
		})
	})
}
