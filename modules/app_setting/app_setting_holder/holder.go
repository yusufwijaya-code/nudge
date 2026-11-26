// Package app_setting_holder keeps the application-wide settings an admin can
// change (Admin › Pengaturan Aplikasi) where every module can read them on each
// request, so a change applies at once, without a restart. It has no
// dependencies, so the report code can read it without importing the admin module.
package app_setting_holder

import (
	"math"
	"sync/atomic"
)

// DefaultDeviationCapPercent is the deviation cap used until an admin sets one.
const DefaultDeviationCapPercent float64 = 12

var deviationCap atomic.Uint64 // math.Float64bits; 0 = never set

// DeviationCapPercent is the deviation a ticket (and a sprint) may have, in
// percent, before it counts as over: the sprint report's "within" check and the
// achievement formula both use it.
func DeviationCapPercent() float64 {
	if bits := deviationCap.Load(); bits != 0 {
		return math.Float64frombits(bits)
	}
	return DefaultDeviationCapPercent
}

// SetDeviationCapPercent installs the cap in force; a value that is not a
// positive number puts the default back.
func SetDeviationCapPercent(v float64) {
	if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
		deviationCap.Store(0)
		return
	}
	deviationCap.Store(math.Float64bits(v))
}
