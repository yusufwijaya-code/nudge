package business_hours_service

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"
)

func loadVendored(t *testing.T) map[string]bool {
	t.Helper()
	cal, err := NewBusinessHoursService().LoadHolidayCalendar("../../../config/holidays_id.json")
	if err != nil {
		t.Fatalf("LoadHolidayCalendar: %v", err)
	}
	return cal.Holidays
}

func daysOf(holidays map[string]bool, year int) []string {
	var out []string
	for d := range holidays {
		if strings.HasPrefix(d, fmt.Sprintf("%d-", year)) {
			out = append(out, d)
		}
	}
	sort.Strings(out)
	return out
}

// The vendored calendar holds the national public holidays only. Cuti bersama
// (the extra days off the government sets) are NOT holidays here: the company
// does not observe them, so those days count as working days.
func TestVendoredHolidayCalendar_2026HasNationalHolidaysAndNoCutiBersama(t *testing.T) {
	holidays := loadVendored(t)
	national := []string{
		"2026-01-01", "2026-01-16", "2026-02-17", "2026-03-19", "2026-03-21", "2026-03-22", "2026-04-03", "2026-04-05",
		"2026-05-01", "2026-05-14", "2026-05-27", "2026-05-31", "2026-06-01", "2026-06-16", "2026-08-17", "2026-08-25", "2026-12-25",
	}
	if got := daysOf(holidays, 2026); strings.Join(got, ",") != strings.Join(national, ",") {
		t.Errorf("2026 = %v, want exactly the 17 national holidays", got)
	}
	for _, day := range []string{"2026-02-16", "2026-03-18", "2026-03-20", "2026-03-23", "2026-03-24", "2026-05-15", "2026-05-28", "2026-12-24"} {
		if holidays[day] {
			t.Errorf("%s is a cuti bersama: the company does not observe it, so it must not be a holiday", day)
		}
	}

	// 24 Dec 2026 is a Thursday: a full working day.
	svc := NewBusinessHoursService()
	wib := time.FixedZone("WIB", 7*3600)
	cal, _ := svc.LoadHolidayCalendar("../../../config/holidays_id.json")
	got := svc.ElapsedBusinessSeconds(time.Date(2026, 12, 24, 8, 30, 0, 0, wib), time.Date(2026, 12, 24, 17, 30, 0, 0, wib), cal)
	if got != 8*3600 {
		t.Errorf("24 Dec counts %d s, want 28800", got)
	}
}

// 2027 is the SKB 3 Menteri of 15 Sep 2026 (Nomor 1205/3/2 Tahun 2026): 18
// national holidays; its 8 cuti bersama are left out.
func TestVendoredHolidayCalendar_2027IsTheOfficialNationalList(t *testing.T) {
	holidays := loadVendored(t)
	national := []string{
		"2027-01-01", "2027-01-05", "2027-02-06", "2027-03-08", "2027-03-10", "2027-03-11", "2027-03-26", "2027-03-28",
		"2027-05-01", "2027-05-06", "2027-05-17", "2027-05-20", "2027-06-01", "2027-06-06", "2027-08-15", "2027-08-17", "2027-12-25", "2027-12-26",
	}
	if got := daysOf(holidays, 2027); strings.Join(got, ",") != strings.Join(national, ",") {
		t.Errorf("2027 = %v, want exactly the 18 national holidays", got)
	}
	for _, day := range []string{"2027-02-05", "2027-03-09", "2027-03-12", "2027-03-15", "2027-03-25", "2027-05-18", "2027-05-19", "2027-12-24"} {
		if holidays[day] {
			t.Errorf("%s is a cuti bersama and must not be a holiday", day)
		}
	}
}

// westernEaster is the Gregorian Easter Sunday (Meeus/Jones/Butcher).
func westernEaster(year int) time.Time {
	a, b, c := year%19, year/100, year%100
	d, e := b/4, b%4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i, k := c/4, c%4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := (h+l-7*m+114)%31 + 1
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

func christianHolidays(year int) []string {
	easter := westernEaster(year)
	f := func(t time.Time) string { return t.Format("2006-01-02") }
	return []string{f(easter.AddDate(0, 0, -2)), f(easter), f(easter.AddDate(0, 0, 39))} // Wafat, Paskah, Kenaikan
}

// The formula reproduces the officially published years; 2028 and 2029 have no
// SKB yet, so they hold only the dates that are certain: the fixed national
// days and the Easter-based ones. The movable Islamic / lunar holidays are added
// when the government publishes the SKB (about September of the year before).
func TestVendoredHolidayCalendar_2028And2029HoldOnlyTheCertainDates(t *testing.T) {
	holidays := loadVendored(t)

	for _, year := range []int{2026, 2027} {
		for _, d := range christianHolidays(year) {
			if !holidays[d] {
				t.Errorf("the Easter formula gives %s for %d but the official list lacks it: the formula is wrong", d, year)
			}
		}
	}
	for _, year := range []int{2028, 2029} {
		want := append([]string{fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-05-01", year), fmt.Sprintf("%d-06-01", year), fmt.Sprintf("%d-08-17", year), fmt.Sprintf("%d-12-25", year)}, christianHolidays(year)...)
		sort.Strings(want)
		if got := daysOf(holidays, year); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%d = %v, want %v", year, got, want)
		}
	}
}
