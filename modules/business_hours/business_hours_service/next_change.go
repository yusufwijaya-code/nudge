package business_hours_service

import (
	"sort"
	"time"

	"nudge/modules/business_hours/business_hours_dto"
)

// NextCountingChange is the next instant after t at which time starts or stops
// counting (a stretch begins or ends: lunch, a Daily Stand Up, the end of the
// day, a weekend or holiday...). ok=false when nothing changes within three
// weeks. The dashboard ticks the clock by itself between two such instants.
func NextCountingChange(t time.Time, cal business_hours_dto.HolidayCalendar) (time.Time, bool) {
	t = t.In(jakartaLocation)
	within := func(x time.Time) bool {
		for _, iv := range workIntervals(x, cal) {
			if !x.Before(iv[0]) && x.Before(iv[1]) {
				return true
			}
		}
		return false
	}
	counting := within(t)
	for d := 0; d <= 21; d++ {
		day := t.AddDate(0, 0, d)
		var bounds []time.Time
		for _, iv := range workIntervals(day, cal) {
			bounds = append(bounds, iv[0], iv[1])
		}
		sort.Slice(bounds, func(i, j int) bool { return bounds[i].Before(bounds[j]) })
		for _, b := range bounds {
			if b.After(t) && within(b) != counting {
				return b, true
			}
		}
	}
	return time.Time{}, false
}
