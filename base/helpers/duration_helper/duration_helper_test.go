package duration_helper

import "testing"

func TestClock_ShowsHoursMinutesSecondsThenDropsTheEmptyLeadingUnits(t *testing.T) {
	cases := map[int64]string{
		0:       "0 detik",
		7:       "7 detik",
		52:      "52 detik",
		59:      "59 detik",
		60:      "1:00",
		61:      "1:01",
		2220:    "37:00",
		2233:    "37:13",
		3599:    "59:59",
		3600:    "1:00:00",
		3661:    "1:01:01",
		8948:    "2:29:08",
		9000:    "2:30:00",
		163956:  "45:32:36",
		-8948:   "2:29:08", // the sign is ignored
		-52:     "52 detik",
		1000000: "277:46:40", // no days: hours just keep counting
	}
	for seconds, want := range cases {
		if got := Clock(seconds); got != want {
			t.Errorf("Clock(%d) = %q, want %q", seconds, got, want)
		}
	}
}
