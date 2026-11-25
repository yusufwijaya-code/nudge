package duration_helper

import "testing"

func TestFormat_WritesWordsAndLeavesOutEveryZeroUnit(t *testing.T) {
	cases := map[int64]string{
		0:       "0 detik",
		7:       "7 detik",
		52:      "52 detik",
		60:      "1 menit",
		61:      "1 menit 1 detik",
		1800:    "30 menit",
		2233:    "37 menit 13 detik",
		3600:    "1 jam",
		3630:    "1 jam 30 detik", // a zero minute in the middle is left out too
		5660:    "1 jam 34 menit 20 detik",
		8948:    "2 jam 29 menit 8 detik",
		9000:    "2 jam 30 menit",
		163956:  "45 jam 32 menit 36 detik",
		-8948:   "2 jam 29 menit 8 detik", // the sign is ignored
		-52:     "52 detik",
		1000000: "277 jam 46 menit 40 detik", // no days: hours just keep counting
	}
	for seconds, want := range cases {
		if got := Format(seconds); got != want {
			t.Errorf("Format(%d) = %q, want %q", seconds, got, want)
		}
	}
}
