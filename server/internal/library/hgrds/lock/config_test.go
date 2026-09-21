package lock

import (
	"testing"
	"time"
)

func TestWithoutWatchDogDoesNotMutateSourceConfig(t *testing.T) {
	base := NewConfig(time.Minute, time.Second)
	fixed := base.WithoutWatchDog()

	if !base.watchDogEnabled {
		t.Fatal("base config watchdog should remain enabled")
	}
	if fixed.watchDogEnabled {
		t.Fatal("fixed lease watchdog should be disabled")
	}
	if fixed.Mutex("fixed").watchDogEnabled {
		t.Fatal("fixed lease lock should not start a watchdog")
	}
	if !base.Mutex("renewable").watchDogEnabled {
		t.Fatal("base config lock should keep its watchdog")
	}
}
