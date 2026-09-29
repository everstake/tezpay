package execute

import (
	"fmt"
	"time"
)

var (
	// batches are valid only for constants.MAX_OPERATION_TTL blocks, confirmation should take a few minutes at most
	batchConfirmationAlertDelay = 30 * time.Minute
	// how often to repeat the alert while still waiting
	batchConfirmationAlertInterval = 30 * time.Minute
)

// watchBatchConfirmation notifies admin if the batch confirmation takes longer than expected
// and keeps reminding until the returned stop function is called.
// If admin was notified, stop also reports the final outcome of the batch.
func watchBatchConfirmation(notify func(string), batchDescription string, opReference string) (stop func(err error)) {
	start := time.Now()
	done := make(chan struct{})
	alerted := make(chan bool, 1)

	go func() {
		wasAlerted := false
		timer := time.NewTimer(batchConfirmationAlertDelay)
		defer timer.Stop()
		for {
			select {
			case <-done:
				alerted <- wasAlerted
				return
			case <-timer.C:
				wasAlerted = true
				notify(fmt.Sprintf("⏳ Batch %s is still waiting for confirmation after %s - tezpay may be stuck: %s", batchDescription, time.Since(start).Round(time.Second), opReference))
				timer.Reset(batchConfirmationAlertInterval)
			}
		}
	}()

	return func(err error) {
		close(done)
		if !<-alerted {
			return
		}
		elapsed := time.Since(start).Round(time.Second)
		if err != nil {
			notify(fmt.Sprintf("❌ Batch %s failed after %s: %s - %s", batchDescription, elapsed, err.Error(), opReference))
			return
		}
		notify(fmt.Sprintf("✅ Batch %s confirmed after %s: %s", batchDescription, elapsed, opReference))
	}
}
