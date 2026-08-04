package conformance

import "time"

const (
	botID = "assent-bot"
	proj  = "platform/orders-service"
	mrIID = "482"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func testClock() fixedClock {
	return fixedClock{t: time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)}
}
