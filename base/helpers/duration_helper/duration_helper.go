package duration_helper

import (
	"fmt"
	"strings"
)

// Format writes a second count in words, leaving out any unit that is zero:
// "1 jam 34 menit 20 detik", "30 menit", "1 jam 30 detik", "52 detik". No days:
// 45 hours is "45 jam". Zero is "0 detik". The sign is ignored (callers say
// "lewat" / "sisa" themselves).
func Format(totalSeconds int64) string {
	if totalSeconds < 0 {
		totalSeconds = -totalSeconds
	}
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60

	var parts []string
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%d jam", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%d menit", minutes))
	}
	if seconds > 0 {
		parts = append(parts, fmt.Sprintf("%d detik", seconds))
	}
	if len(parts) == 0 {
		return "0 detik"
	}
	return strings.Join(parts, " ")
}
