// Package app_setting is Admin › Pengaturan Aplikasi: values that used to be
// hard-coded (the deviation cap) and that an admin can now change.
package app_setting

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"nudge/modules/app_setting/app_setting_holder"
)

const (
	keyDeviationCap = "deviation_cap_percent"
	// MinDeviationCap / MaxDeviationCap bound what an admin may enter.
	MinDeviationCap float64 = 1
	MaxDeviationCap float64 = 100
)

// ValidationError: the value is not acceptable (400).
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

// Settings is what the admin page shows.
type Settings struct {
	DeviationCapPercent float64    `json:"deviationCapPercent"`
	DefaultPercent      float64    `json:"defaultPercent"`
	UpdatedBy           string     `json:"updatedBy,omitempty"`
	UpdatedDate         *time.Time `json:"updatedDate,omitempty"`
}

type Service struct {
	repo Repository
	// apply installs the cap in force; defaults to the shared holder.
	apply func(float64)
	// current reads the cap in force.
	current func() float64
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, apply: app_setting_holder.SetDeviationCapPercent, current: app_setting_holder.DeviationCapPercent}
}

// Load installs the stored values (run at start-up). A bad stored value falls
// back to the default rather than stopping the app.
func (s *Service) Load(ctx context.Context) error {
	row, err := s.repo.Get(ctx, keyDeviationCap)
	if err != nil {
		return fmt.Errorf("app_setting: load: %w", err)
	}
	if row == nil {
		s.apply(0)
		return nil
	}
	v, err := strconv.ParseFloat(row.Value, 64)
	if err != nil || v < MinDeviationCap || v > MaxDeviationCap {
		s.apply(0)
		return nil
	}
	s.apply(v)
	return nil
}

func (s *Service) Get(ctx context.Context) (Settings, error) {
	row, err := s.repo.Get(ctx, keyDeviationCap)
	if err != nil {
		return Settings{}, err
	}
	out := Settings{DeviationCapPercent: s.current(), DefaultPercent: app_setting_holder.DefaultDeviationCapPercent}
	if row != nil {
		out.UpdatedBy = row.UpdatedBy
		t := row.UpdatedDate
		out.UpdatedDate = &t
	}
	return out, nil
}

// SetDeviationCap saves the cap and applies it at once.
func (s *Service) SetDeviationCap(ctx context.Context, percent float64, editor string) (Settings, error) {
	if math.IsNaN(percent) || math.IsInf(percent, 0) || percent < MinDeviationCap || percent > MaxDeviationCap {
		return Settings{}, &ValidationError{Message: fmt.Sprintf("Deviasi harus antara %.0f%% dan %.0f%%", MinDeviationCap, MaxDeviationCap)}
	}
	percent = math.Round(percent*100) / 100
	if err := s.repo.Set(ctx, keyDeviationCap, strconv.FormatFloat(percent, 'f', -1, 64), editor); err != nil {
		return Settings{}, err
	}
	s.apply(percent)
	return s.Get(ctx)
}
