package duration_helper

import "fmt"

// Clock renders a second count the way a stopwatch does: H:MM:SS, or M:SS when
// there is no full hour, or just "S detik" when there is no full minute either.
// No days: 45 hours is "45:00:00". The sign is ignored (callers say "lewat" /
// "sisa" themselves).
//
//	8948 -> "2:29:08"    2220 -> "37:00"    52 -> "52 detik"    0 -> "0 detik"
func Clock(totalSeconds int64) string {
	if totalSeconds < 0 {
		totalSeconds = -totalSeconds
	}
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	switch {
	case hours > 0:
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	case minutes > 0:
		return fmt.Sprintf("%d:%02d", minutes, seconds)
	}
	return fmt.Sprintf("%d detik", seconds)
}
