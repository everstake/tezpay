package execute

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type notificationRecorder struct {
	mu       sync.Mutex
	messages []string
}

func (r *notificationRecorder) notify(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, msg)
}

func (r *notificationRecorder) get() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.messages...)
}

func withConfirmationAlertTiming(t *testing.T, delay, interval time.Duration) {
	originalDelay, originalInterval := batchConfirmationAlertDelay, batchConfirmationAlertInterval
	batchConfirmationAlertDelay, batchConfirmationAlertInterval = delay, interval
	t.Cleanup(func() {
		batchConfirmationAlertDelay, batchConfirmationAlertInterval = originalDelay, originalInterval
	})
}

func TestWatchBatchConfirmationNoAlertWhenConfirmedQuickly(t *testing.T) {
	withConfirmationAlertTiming(t, 200*time.Millisecond, 200*time.Millisecond)
	recorder := &notificationRecorder{}

	stop := watchBatchConfirmation(recorder.notify, "1/2 (#1000)", "https://tzkt.io/op")
	stop(nil)

	time.Sleep(300 * time.Millisecond)
	assert.Empty(t, recorder.get())
}

func TestWatchBatchConfirmationAlertsAndReportsConfirmation(t *testing.T) {
	withConfirmationAlertTiming(t, 20*time.Millisecond, 50*time.Millisecond)
	recorder := &notificationRecorder{}

	stop := watchBatchConfirmation(recorder.notify, "2/136 (#1000)", "https://tzkt.io/op")
	time.Sleep(100 * time.Millisecond)
	stop(nil)

	messages := recorder.get()
	assert.GreaterOrEqual(t, len(messages), 2)
	assert.Contains(t, messages[0], "still waiting for confirmation")
	assert.Contains(t, messages[0], "2/136 (#1000)")
	assert.Contains(t, messages[0], "https://tzkt.io/op")
	assert.Contains(t, messages[len(messages)-1], "confirmed after")

	// no more reminders after stop
	count := len(messages)
	time.Sleep(100 * time.Millisecond)
	assert.Len(t, recorder.get(), count)
}

func TestWatchBatchConfirmationReportsFailureAfterAlert(t *testing.T) {
	withConfirmationAlertTiming(t, 20*time.Millisecond, time.Hour)
	recorder := &notificationRecorder{}

	stop := watchBatchConfirmation(recorder.notify, "2/136 (#1000)", "https://tzkt.io/op")
	time.Sleep(50 * time.Millisecond)
	stop(errors.New("operation failed"))

	messages := recorder.get()
	assert.Len(t, messages, 2)
	assert.Contains(t, messages[1], "failed after")
	assert.Contains(t, messages[1], "operation failed")
}
