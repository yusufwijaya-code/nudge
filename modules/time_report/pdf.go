package time_report

import (
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
)

type rgb struct{ r, g, b int }

var (
	cInk      = rgb{28, 30, 52}
	cMuted    = rgb{102, 108, 128}
	cFaint    = rgb{150, 156, 172}
	cBorder   = rgb{226, 229, 237}
	cSurface  = rgb{246, 247, 251}
	cPrimary  = rgb{88, 76, 230}
	cPrimary2 = rgb{55, 48, 163}
	cPrimSoft = rgb{238, 236, 255}
	cGreen    = rgb{21, 128, 61}
	cGreenBg  = rgb{232, 247, 237}
	cRed      = rgb{190, 30, 45}
	cRedBg    = rgb{253, 237, 238}
	cAmber    = rgb{180, 99, 10}
	cAmberBg  = rgb{255, 245, 230}
	cWhite    = rgb{255, 255, 255}
	cHeadRow  = rgb{43, 45, 78}
)

const (
	pageW   = 210.0
	pageH   = 297.0
	margin  = 14.0
	content = pageW - 2*margin
	bottom  = 18.0
)

type doc struct {
	pdf *fpdf.Fpdf
	tr  func(string) string
	r   *Report
}

func (d *doc) fill(c rgb) { d.pdf.SetFillColor(c.r, c.g, c.b) }
func (d *doc) text(c rgb) { d.pdf.SetTextColor(c.r, c.g, c.b) }
func (d *doc) draw(c rgb) { d.pdf.SetDrawColor(c.r, c.g, c.b) }
func (d *doc) font(style string, size float64) {
	d.pdf.SetFont("Helvetica", style, size)
}

// cell writes one line of text in a box of width w (no wrapping, cut with "…").
func (d *doc) cell(w, h float64, s, align string, fill bool) {
	d.pdf.CellFormat(w, h, d.tr(d.fit(s, w-2)), "", 0, align, fill, 0, "")
}

// fit shortens s with "…" until it fits width w (mm) in the current font.
func (d *doc) fit(s string, w float64) string {
	if d.pdf.GetStringWidth(d.tr(s)) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 1 {
		r = r[:len(r)-1]
		if d.pdf.GetStringWidth(d.tr(string(r)+"…")) <= w {
			return string(r) + "…"
		}
	}
	return "…"
}

// lines wraps s to width w (mm) in the current font, at spaces; a word longer than
// the line is cut. The lines are untranslated text.
func (d *doc) lines(s string, w float64) []string {
	width := func(t string) float64 { return d.pdf.GetStringWidth(d.tr(t)) }
	var out []string
	cur := ""
	for _, word := range strings.Fields(s) {
		next := word
		if cur != "" {
			next = cur + " " + word
		}
		if width(next) <= w {
			cur = next
			continue
		}
		if cur != "" {
			out = append(out, cur)
		}
		for width(word) > w { // a single word wider than the line
			r := []rune(word)
			n := len(r)
			for n > 1 && width(string(r[:n])) > w {
				n--
			}
			out = append(out, string(r[:n]))
			word = string(r[n:])
		}
		cur = word
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// para writes s wrapped to width w at x, one line every lh mm, and moves below it.
func (d *doc) para(x, w, lh float64, s string) {
	for _, l := range d.lines(s, w) {
		d.pdf.SetX(x)
		d.pdf.CellFormat(w, lh, d.tr(l), "", 1, "L", false, 0, "")
	}
}

// space starts a new page when h mm no longer fit.
func (d *doc) space(h float64) {
	if d.pdf.GetY()+h > pageH-bottom {
		d.pdf.AddPage()
	}
}

// ---- formatting --------------------------------------------------------------

var months = []string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

func dayLabel(isoDay string) string {
	t, err := time.ParseInLocation("2006-01-02", isoDay, wib)
	if err != nil {
		return isoDay
	}
	return fmt.Sprintf("%d %s %d", t.Day(), months[t.Month()-1], t.Year())
}

func stampLabel(rfc string, withTime bool) string {
	t, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		return ""
	}
	t = t.In(wib)
	if withTime {
		return fmt.Sprintf("%d %s %d, %02d.%02d WIB", t.Day(), months[t.Month()-1], t.Year(), t.Hour(), t.Minute())
	}
	return fmt.Sprintf("%d %s %d", t.Day(), months[t.Month()-1], t.Year())
}

// hm is "3j 05m", "45m", "0m" (hours and minutes; the sign is kept).
func hm(seconds int64) string {
	sign := ""
	if seconds < 0 {
		sign, seconds = "-", -seconds
	}
	minutes := (seconds + 30) / 60
	h, m := minutes/60, minutes%60
	switch {
	case h == 0:
		return fmt.Sprintf("%s%dm", sign, m)
	case m == 0:
		return fmt.Sprintf("%s%dj", sign, h)
	}
	return fmt.Sprintf("%s%dj %02dm", sign, h, m)
}

// signed is hm with a "+" for more than the estimate ("+20m", "-1j 05m").
func signed(seconds int64) string {
	if seconds > 0 {
		return "+" + hm(seconds)
	}
	return hm(seconds)
}

// hours is "12,50 jam".
func hours(seconds int64) string {
	return strings.Replace(fmt.Sprintf("%.2f jam", float64(seconds)/3600), ".", ",", 1)
}

func pct(v float64) string {
	return strings.Replace(fmt.Sprintf("%.2f%%", v), ".", ",", 1)
}

func pct1(v float64) string {
	if v == math.Trunc(v) {
		return fmt.Sprintf("%.0f%%", v)
	}
	return strings.Replace(fmt.Sprintf("%.1f%%", v), ".", ",", 1)
}

func sp(v float64) string {
	if v == math.Trunc(v) {
		return fmt.Sprintf("%.0f SP", v)
	}
	return strings.Replace(fmt.Sprintf("%.1f SP", v), ".", ",", 1)
}

func (d *doc) bandText() string {
	return fmt.Sprintf("Skor %s–%s", pct1(100-d.r.CapPercent), pct1(100+d.r.CapPercent))
}

// ---- page parts -------------------------------------------------------------

func (d *doc) cover() {
	p := d.pdf
	// Indigo band with a lighter stripe.
	d.fill(cPrimary2)
	p.Rect(0, 0, pageW, 46, "F")
	d.fill(cPrimary)
	p.Rect(0, 40, pageW, 6, "F")

	d.text(rgb{199, 196, 255})
	d.font("B", 8)
	p.SetXY(margin, 11)
	d.cell(content, 4, "NUDGE  ·  LAPORAN CASCADING KEY RESULT", "L", false)
	d.text(cWhite)
	d.font("B", 23)
	p.SetXY(margin, 17)
	d.cell(content, 10, "Estimasi vs Waktu Aktual", "L", false)
	d.font("", 10)
	p.SetXY(margin, 29)
	d.cell(content, 5, fmt.Sprintf("%s  ·  %s  ·  Tiket dibuat %s – %s", d.r.UserName, d.r.BoardName, dayLabel(d.r.From), dayLabel(d.r.To)), "L", false)
	p.SetY(54)
}

// meta is the key/value box under the cover.
func (d *doc) meta() {
	p := d.pdf
	rows := [][2]string{
		{"Pemilik", fmt.Sprintf("%s (%s)", d.r.UserName, d.r.UserEmail)},
		{"Board", d.r.BoardName + projectsNote(d.r.Projects)},
		{"Periode", fmt.Sprintf("Tiket yang dibuat %s sampai %s", dayLabel(d.r.From), dayLabel(d.r.To))},
		{"Key Result", fmt.Sprintf("Jaga selisih Waktu Aktual dari Estimasi dalam ±%s (%s).", pct1(d.r.CapPercent), d.bandText())},
		{"Rumus", fmt.Sprintf("Skor = Waktu dihitung ÷ Estimasi × 100%%. Deviasi = |Waktu dihitung – Estimasi| ÷ Estimasi × 100%%. Sesuai target bila deviasi <= %s.", pct1(d.r.CapPercent))},
		{"Sumber data", "Riwayat status tiket di Jira, dibaca ulang saat laporan ini dibuat. Estimasi = Story Point × 1 jam."},
		{"Dibuat", stampLabel(d.r.GeneratedAt, true)},
	}
	const labelW, padX, lineH = 34.0, 4.0, 4.6
	valueW := content - labelW - 2*padX - 3
	// Measure first so the box fits on the page as one piece.
	d.font("", 9)
	total := 4.0
	heights := make([]float64, len(rows))
	for i, r := range rows {
		n := len(d.lines(r[1], valueW))
		if n < 1 {
			n = 1
		}
		heights[i] = float64(n)*lineH + 3
		total += heights[i]
	}
	d.space(total + 4)
	top := p.GetY()
	d.fill(cSurface)
	d.draw(cBorder)
	p.RoundedRect(margin, top, content, total, 2.5, "1234", "FD")

	y := top + 2
	for i, r := range rows {
		d.font("B", 9)
		d.text(cInk)
		p.SetXY(margin+padX, y+1.5)
		d.cell(labelW, lineH, r[0], "L", false)
		d.font("", 9)
		d.text(cInk)
		p.SetXY(margin+padX+labelW, y+1.5)
		d.para(margin+padX+labelW, valueW, lineH, r[1])
		y += heights[i]
		if i < len(rows)-1 {
			d.draw(cBorder)
			p.Line(margin+padX, y, margin+content-padX, y)
		}
	}
	p.SetY(top + total + 6)
}

func projectsNote(projects []string) string {
	if len(projects) == 0 {
		return ""
	}
	return " (project " + strings.Join(projects, ", ") + ")"
}

type tile struct {
	label, value, sub string
	fg, bg            rgb
}

func (d *doc) tiles(ts []tile, h float64) {
	p := d.pdf
	gap := 4.0
	w := (content - gap*float64(len(ts)-1)) / float64(len(ts))
	d.space(h + 4)
	top := p.GetY()
	for i, t := range ts {
		x := margin + float64(i)*(w+gap)
		d.fill(t.bg)
		d.draw(cBorder)
		p.RoundedRect(x, top, w, h, 2.5, "1234", "FD")
		d.text(cMuted)
		d.font("B", 6.8)
		p.SetXY(x+4, top+3.5)
		d.cell(w-6, 3.5, strings.ToUpper(t.label), "L", false)
		d.text(t.fg)
		d.font("B", 17)
		p.SetXY(x+4, top+8.5)
		d.cell(w-6, 8, t.value, "L", false)
		d.text(cMuted)
		d.font("", 7.5)
		p.SetXY(x+4, top+h-6.5)
		d.cell(w-6, 4, t.sub, "L", false)
	}
	p.SetY(top + h + 5)
}

// heading writes a section title; keep mm of what follows stay on the same page.
func (d *doc) heading(n, title, sub string, keep float64) {
	p := d.pdf
	d.font("", 8.5)
	d.space(18 + float64(len(d.lines(sub, content)))*4.2 + keep)
	d.text(cPrimary)
	d.font("B", 8)
	p.SetX(margin)
	d.cell(content, 4, "BAGIAN "+n, "L", false)
	p.Ln(4.5)
	d.text(cInk)
	d.font("B", 13)
	p.SetX(margin)
	d.cell(content, 6.5, title, "L", false)
	p.Ln(7)
	if sub != "" {
		d.text(cMuted)
		d.font("", 8.5)
		d.para(margin, content, 4.2, sub)
	}
	p.Ln(2.5)
}

func scoreColors(within bool) (rgb, rgb) {
	if within {
		return cGreen, cGreenBg
	}
	return cRed, cRedBg
}

func (d *doc) summaryTiles() {
	s := d.r.Summary
	achFg, achBg := cGreen, cGreenBg
	switch {
	case s.ScoredCount == 0:
		achFg, achBg = cMuted, cSurface
	case s.AchievementPercent < 100 && s.AchievementPercent >= 50:
		achFg, achBg = cAmber, cAmberBg
	case s.AchievementPercent < 50:
		achFg, achBg = cRed, cRedBg
	}
	scoreFg, scoreBg := scoreColors(s.Within)
	devFg, devBg := scoreColors(s.Within)
	if s.ScoredCount == 0 {
		scoreFg, scoreBg, devFg, devBg = cMuted, cSurface, cMuted, cSurface
	}
	dash := func(v string) string {
		if s.ScoredCount == 0 {
			return "–"
		}
		return v
	}
	d.tiles([]tile{
		{"Achievement", dash(pct1(s.AchievementPercent)), "Batas " + pct1(d.r.CapPercent) + " ÷ deviasi total", achFg, achBg},
		{"Skor (aktual ÷ estimasi)", dash(pct(s.ScorePercent)), d.bandText(), scoreFg, scoreBg},
		{"Deviasi total", dash(pct(s.AggregateDeviationPercent)), "Batas ±" + pct1(d.r.CapPercent), devFg, devBg},
		{"Tiket sesuai target", fmt.Sprintf("%d/%d", s.WithinCount, s.ScoredCount), dash(pct1(s.WithinPercent)) + " dari tiket yang dinilai", cPrimary, cPrimSoft},
	}, 27)
	d.tiles([]tile{
		{"Estimasi", hm(s.EstimateSeconds), hours(s.EstimateSeconds), cInk, cSurface},
		{"Waktu kotor", hm(s.RawSeconds), "Total di status yang dihitung", cInk, cSurface},
		{"Waktu dihitung", hm(s.CountedSeconds), "Setelah dikurangi " + hm(s.DeductedSeconds), cPrimary, cPrimSoft},
	}, 24)
}

// waterfall: raw time, each deduction, counted time, with bars.
func (d *doc) waterfall() {
	p := d.pdf
	s := d.r.Summary
	d.heading("1", "Dari waktu kotor ke waktu dihitung",
		"Semua tiket yang dinilai. Waktu kotor adalah lama tiket berada di status yang dihitung, tanpa potongan apa pun. Setiap potongan di bawah dihitung sekali per detik, dengan urutan: hari libur, di luar jam kerja (08.30–17.30), istirahat (12.00–13.00), lalu jadwal board.", 7.4*4)
	type row struct {
		label    string
		seconds  int64
		kind     string // raw | ded | total
		category string
	}
	rows := []row{{"Waktu kotor", s.RawSeconds, "raw", ""}}
	for _, ded := range d.r.Deductions {
		label := ded.Label
		if ded.Category == CatSlot {
			label = ded.Label + " (jadwal board)"
		}
		rows = append(rows, row{label, ded.Seconds, "ded", ded.Category})
	}
	rows = append(rows, row{"Waktu dihitung", s.CountedSeconds, "total", ""})

	const rowH, labelW, valueW, pctW = 7.4, 58.0, 24.0, 18.0
	barW := content - labelW - valueW - pctW - 6
	d.space(rowH*float64(len(rows)) + 6)
	top := p.GetY()
	d.draw(cBorder)
	d.fill(cWhite)
	p.RoundedRect(margin, top, content, rowH*float64(len(rows))+2, 2.5, "1234", "D")
	y := top + 1
	for i, r := range rows {
		if r.kind != "ded" {
			if r.kind == "total" {
				d.fill(cPrimSoft)
			} else {
				d.fill(cSurface)
			}
			p.Rect(margin+0.4, y, content-0.8, rowH, "F")
		}
		prefix := "– "
		style, color := "", cInk
		switch r.kind {
		case "raw":
			prefix, style = "", "B"
		case "total":
			prefix, style, color = "= ", "B", cPrimary2
		}
		d.text(color)
		d.font(style, 9)
		p.SetXY(margin+3, y+1.6)
		d.cell(labelW, 4, prefix+r.label, "L", false)
		p.SetXY(margin+3+labelW, y+1.6)
		val := hm(r.seconds)
		if r.kind == "ded" {
			val = "–" + val
		}
		d.cell(valueW, 4, val, "R", false)

		// The bar: share of the raw time.
		share := 0.0
		if s.RawSeconds > 0 {
			share = float64(r.seconds) / float64(s.RawSeconds)
		}
		bx := margin + 3 + labelW + valueW + 3
		d.fill(cSurface)
		p.RoundedRect(bx, y+2.4, barW, 2.6, 1.2, "1234", "F")
		switch r.kind {
		case "raw":
			d.fill(cFaint)
		case "total":
			d.fill(cPrimary)
		default:
			d.fill(dedColor(r.category))
		}
		if w := barW * share; w > 0.6 {
			p.RoundedRect(bx, y+2.4, w, 2.6, 1.2, "1234", "F")
		}
		d.text(cMuted)
		d.font("", 8)
		p.SetXY(bx+barW, y+1.6)
		d.cell(pctW, 4, pct1(math.Round(share*1000)/10), "R", false)
		y += rowH
		if i < len(rows)-1 && rows[i+1].kind == "ded" && r.kind == "ded" {
			d.draw(cBorder)
			p.Line(margin+3, y, margin+content-3, y)
		}
	}
	p.SetY(top + rowH*float64(len(rows)) + 7)
}

func dedColor(category string) rgb {
	switch category {
	case CatOutside:
		return rgb{148, 163, 184}
	case CatLunch:
		return rgb{245, 158, 11}
	case CatWeekend:
		return rgb{100, 116, 139}
	case CatHoliday:
		return rgb{229, 72, 77}
	case CatLeave:
		return rgb{59, 130, 246}
	}
	return rgb{139, 92, 246}
}

// ticketTable is one row per scored ticket.
func (d *doc) ticketTable() {
	p := d.pdf
	d.heading("2", "Ringkasan per tiket",
		fmt.Sprintf("%d tiket dinilai · %d sesuai target (%s). Urut berdasarkan tanggal tiket dibuat.", d.r.Summary.ScoredCount, d.r.Summary.WithinCount, pct1(d.r.Summary.WithinPercent)), 7*3)
	cols := []struct {
		title string
		w     float64
		align string
	}{
		{"Tiket", 22, "L"}, {"Judul", 50, "L"}, {"Estimasi", 16, "R"}, {"Kotor", 16, "R"}, {"Potongan", 18, "R"},
		{"Dihitung", 17, "R"}, {"Deviasi", 15, "R"}, {"Skor", 14, "R"}, {"Hasil", 14, "C"},
	}
	const rowH = 7.0
	header := func() {
		d.fill(cHeadRow)
		d.text(cWhite)
		d.font("B", 7.6)
		p.SetX(margin)
		for _, c := range cols {
			p.CellFormat(c.w, rowH, d.tr(c.title), "", 0, c.align, true, 0, "")
		}
		p.Ln(rowH)
	}
	d.space(rowH * 3)
	header()
	for i, t := range d.r.Tickets {
		if p.GetY()+rowH > pageH-bottom {
			p.AddPage()
			header()
		}
		if i%2 == 1 {
			d.fill(cSurface)
		} else {
			d.fill(cWhite)
		}
		fg, _ := scoreColors(t.Within)
		result := "DI LUAR"
		if t.Within {
			result = "SESUAI"
		}
		vals := []string{t.Key, t.Summary, hm(t.EstimateSeconds), hm(t.RawSeconds), hm(t.DeductedSeconds), hm(t.CountedSeconds), pct1(math.Round(t.DeviationPercent*10) / 10), pct1(math.Round(t.ScorePercent*10) / 10), result}
		p.SetX(margin)
		for c, col := range cols {
			switch c {
			case 0:
				d.text(cPrimary2)
				d.font("B", 8)
			case 6, 7:
				d.text(fg)
				d.font("", 8)
			case 8:
				d.text(fg)
				d.font("B", 7.6)
			default:
				d.text(cInk)
				d.font("", 8)
			}
			p.CellFormat(col.w, rowH, d.tr(d.fit(vals[c], col.w-2)), "", 0, col.align, true, 0, "")
		}
		p.Ln(rowH)
	}
	// Total row.
	s := d.r.Summary
	d.fill(cPrimSoft)
	d.text(cPrimary2)
	d.font("B", 8)
	p.SetX(margin)
	tot := []string{"TOTAL", "", hm(s.EstimateSeconds), hm(s.RawSeconds), hm(s.DeductedSeconds), hm(s.CountedSeconds), pct1(math.Round(s.AggregateDeviationPercent*10) / 10), pct1(math.Round(s.ScorePercent*10) / 10), map[bool]string{true: "SESUAI", false: "DI LUAR"}[s.Within]}
	if s.ScoredCount == 0 {
		tot[6], tot[7], tot[8] = "–", "–", "–"
	}
	for c, col := range cols {
		p.CellFormat(col.w, rowH, d.tr(tot[c]), "", 0, col.align, true, 0, "")
	}
	p.Ln(rowH + 6)
}

// ticketCards are the per-ticket details.
func (d *doc) ticketCards() {
	p := d.pdf
	d.heading("3", "Rincian per tiket",
		"Untuk setiap tiket: waktu kotor per status, setiap potongan beserta nilainya, waktu dihitung, lalu perbandingannya dengan estimasi.", 60)
	for _, t := range d.r.Tickets {
		fg, bg := scoreColors(t.Within)
		type line struct {
			label, value string
			strong       bool
			color        rgb
		}
		var ls []line
		for _, st := range t.StatusTimes {
			ls = append(ls, line{"Waktu kotor di " + st.Status, hm(st.RawSeconds), false, cInk})
		}
		if len(t.StatusTimes) > 1 {
			ls = append(ls, line{"Waktu kotor", hm(t.RawSeconds), true, cInk})
		}
		for _, ded := range t.Deductions {
			ls = append(ls, line{"– " + ded.Label, "–" + hm(ded.Seconds), false, cMuted})
		}
		ls = append(ls, line{"= Waktu dihitung", hm(t.CountedSeconds), true, cPrimary2})

		const lineH, headH = 5.0, 21.0
		h := headH + float64(len(ls))*lineH + 6
		d.space(h + 4)
		top := p.GetY()
		d.fill(bg)
		d.draw(cBorder)
		p.RoundedRect(margin, top, content, h, 2.5, "1234", "FD")

		// Header: key, result badge, title, facts.
		d.text(cPrimary2)
		d.font("B", 10.5)
		p.SetXY(margin+5, top+3.5)
		d.cell(60, 5, t.Key, "L", false)
		badge := "SESUAI TARGET"
		if !t.Within {
			badge = "DI LUAR TARGET"
		}
		d.font("B", 7.5)
		bw := p.GetStringWidth(badge) + 6
		d.fill(fg)
		p.RoundedRect(margin+content-bw-4, top+3.3, bw, 5.2, 2.4, "1234", "F")
		d.text(cWhite)
		p.SetXY(margin+content-bw-4, top+3.6)
		d.cell(bw, 4.6, badge, "C", false)

		d.text(cInk)
		d.font("", 9.5)
		p.SetXY(margin+5, top+9)
		d.cell(content-10, 5, t.Summary, "L", false)
		facts := []string{t.Type, "Status " + t.Status, "Dibuat " + stampLabel(t.Created, false)}
		if len(t.Sprints) > 0 {
			facts = append(facts, "Sprint "+strings.Join(t.Sprints, ", "))
		}
		if t.Running {
			facts = append(facts, "masih berjalan")
		}
		d.text(cMuted)
		d.font("", 7.8)
		p.SetXY(margin+5, top+14.2)
		d.cell(content-10, 4, strings.Join(nonEmpty(facts), "  ·  "), "L", false)

		// Left: the breakdown. Right: the comparison.
		leftW := content*0.58 - 5
		y := top + headH
		for _, l := range ls {
			d.text(l.color)
			if l.strong {
				d.font("B", 8.6)
			} else {
				d.font("", 8.6)
			}
			p.SetXY(margin+5, y)
			d.cell(leftW-26, lineH, l.label, "L", false)
			p.SetXY(margin+5+leftW-26, y)
			d.cell(26, lineH, l.value, "R", false)
			y += lineH
		}

		rx := margin + content*0.58 + 4
		rw := content - (rx - margin) - 5
		d.draw(cBorder)
		p.Line(rx-4, top+headH, rx-4, top+h-4)
		pairs := [][2]string{
			{"Estimasi", fmt.Sprintf("%s (%s)", hm(t.EstimateSeconds), sp(t.StoryPoints))},
			{"Waktu dihitung", hm(t.CountedSeconds)},
			{"Selisih", signed(t.CountedSeconds - t.EstimateSeconds)},
			{"Deviasi", pct(t.DeviationPercent)},
			{"Skor", pct(t.ScorePercent)},
		}
		yy := top + headH
		for i, pr := range pairs {
			d.text(cMuted)
			d.font("", 8.6)
			p.SetXY(rx, yy)
			d.cell(rw/2, lineH, pr[0], "L", false)
			if i >= 3 {
				d.text(fg)
				d.font("B", 8.6)
			} else {
				d.text(cInk)
				d.font("B", 8.6)
			}
			p.SetXY(rx+rw/2, yy)
			d.cell(rw/2, lineH, pr[1], "R", false)
			yy += lineH
		}
		p.SetY(top + h + 4)
	}
}

func nonEmpty(in []string) []string {
	var out []string
	for _, s := range in {
		if strings.TrimSpace(s) != "" && s != "Dibuat " {
			out = append(out, s)
		}
	}
	return out
}

func (d *doc) excludedAndNotes() {
	p := d.pdf
	n := "4"
	if len(d.r.Excluded) > 0 {
		d.heading(n, "Tidak dinilai", "Tiket dalam periode ini yang tidak ikut skor, beserta alasannya.", 10)
		for _, e := range d.r.Excluded {
			text := fmt.Sprintf("%s — %s. %s.", e.Key, e.Summary, e.Reason)
			if e.CountedSeconds > 0 {
				text += " Waktu dihitung " + hm(e.CountedSeconds) + "."
			}
			d.font("", 8.6)
			ls := d.lines(text, content-8)
			d.space(float64(len(ls))*4.4 + 3)
			d.text(cRed)
			d.font("B", 9)
			p.SetX(margin)
			d.cell(5, 4.4, "•", "L", false)
			d.text(cInk)
			d.font("", 8.6)
			p.SetX(margin + 5)
			d.para(margin+5, content-8, 4.4, text)
			p.Ln(1)
		}
		p.Ln(3)
		n = "5"
	}
	d.heading(n, "Cara membaca laporan ini", "", 12)
	statuses := []string{}
	for _, pk := range d.r.Projects {
		if st := d.r.Statuses[pk]; len(st) > 0 {
			statuses = append(statuses, pk+": "+strings.Join(st, ", "))
		}
	}
	notes := []string{
		"Waktu kotor = lama tiket berada di status yang dihitung (" + strings.Join(statuses, "; ") + "), dibaca dari riwayat status Jira. Tiket yang masih di status itu dihitung sampai laporan dibuat.",
		"Jam kerja 08.30–17.30 WIB, Senin–Jumat, dengan istirahat 12.00–13.00. Waktu di luar itu tidak dihitung.",
		"Akhir pekan, libur nasional, dan hari cuti yang tercatat di Nudge tidak dihitung, sepanjang harinya.",
		"Jadwal board (Daily Stand Up dan kegiatan khusus di Aturan Board) tidak dihitung pada jam dan harinya; yang hanya berlaku di minggu ke-N mengikuti sprint tiket pada hari itu.",
		fmt.Sprintf("Estimasi = Story Point × 1 jam. Tiket sesuai target bila deviasinya <= %s (%s). Achievement = 100%% bila deviasi total <= %s, selain itu %s ÷ deviasi total × 100%%.", pct1(d.r.CapPercent), d.bandText(), pct1(d.r.CapPercent), pct1(d.r.CapPercent)),
		"Laporan ini dihitung ulang dari Jira setiap kali dibuka atau diunduh, jadi angkanya selalu yang terbaru.",
	}
	if d.r.Truncated {
		notes = append(notes, fmt.Sprintf("Periode ini punya lebih dari %d tiket; hanya %d tiket pertama yang dibaca. Persempit rentang tanggal untuk melihat semuanya.", MaxTickets, MaxTickets))
	}
	if d.r.SyncWarning != "" {
		notes = append(notes, d.r.SyncWarning)
	}
	for _, note := range notes {
		d.font("", 8.6)
		ls := d.lines(note, content-8)
		d.space(float64(len(ls))*4.4 + 2)
		d.text(cPrimary)
		d.font("B", 9)
		p.SetX(margin)
		d.cell(5, 4.4, "•", "L", false)
		d.text(cInk)
		d.font("", 8.6)
		p.SetX(margin + 5)
		d.para(margin+5, content-8, 4.4, note)
		p.Ln(1)
	}
}

// lookalikes are characters the built-in fonts cannot show, with what to show instead.
var lookalikes = strings.NewReplacer("−", "-", "≤", "<=", "≥", ">=", "→", "->", "←", "<-", "✓", "v", "✔", "v", " ", " ")

// safeTranslator converts to the fonts' code page; a character it cannot show (an
// emoji in a Jira title, say) becomes "?" instead of broken bytes.
func safeTranslator(tr func(string) string) func(string) string {
	ok := map[rune]bool{}
	return func(s string) string {
		s = lookalikes.Replace(s)
		var b strings.Builder
		for _, r := range s {
			if r < 128 {
				b.WriteRune(r)
				continue
			}
			good, seen := ok[r]
			if !seen {
				// fpdf turns a character it does not know into "."
				out := tr(string(r))
				good = len(out) == 1 && out != "."
				ok[r] = good
			}
			if good {
				b.WriteRune(r)
			} else {
				// dropped (an emoji, say): nothing readable can stand in for it
			}
		}
		return tr(strings.Join(strings.Fields(b.String()), " "))
	}
}

// RenderPDF writes the report as a PDF to w.
func RenderPDF(w io.Writer, r *Report) error {
	p := fpdf.New("P", "mm", "A4", "")
	d := &doc{pdf: p, tr: safeTranslator(p.UnicodeTranslatorFromDescriptor("")), r: r}
	p.SetMargins(margin, 16, margin)
	p.SetAutoPageBreak(true, bottom)
	p.AliasNbPages("{nb}")
	p.SetTitle(d.tr("Laporan Estimasi vs Waktu Aktual — "+r.UserName), false)
	p.SetAuthor("Nudge", false)
	p.SetCreator("Nudge", false)

	p.SetHeaderFuncMode(func() {
		if p.PageNo() == 1 {
			return
		}
		d.text(cMuted)
		d.font("", 7.5)
		p.SetXY(margin, 8)
		d.cell(content/2, 4, "Laporan Estimasi vs Waktu Aktual · "+r.UserName, "L", false)
		p.SetXY(margin+content/2, 8)
		d.cell(content/2, 4, r.BoardName+" · "+dayLabel(r.From)+" – "+dayLabel(r.To), "R", false)
		d.draw(cBorder)
		p.Line(margin, 13, margin+content, 13)
		p.SetY(16)
	}, true)
	p.SetFooterFunc(func() {
		d.draw(cBorder)
		p.Line(margin, pageH-12, margin+content, pageH-12)
		d.text(cFaint)
		d.font("", 7.5)
		p.SetXY(margin, pageH-10.5)
		d.cell(content/2, 4, "Nudge · dibuat "+stampLabel(r.GeneratedAt, true), "L", false)
		p.SetXY(margin+content/2, pageH-10.5)
		p.CellFormat(content/2, 4, fmt.Sprintf("Halaman %d dari {nb}", p.PageNo()), "", 0, "R", false, 0, "")
	})

	p.AddPage()
	d.cover()
	d.meta()
	d.summaryTiles()
	if len(r.Tickets) > 0 {
		d.waterfall()
		d.ticketTable()
		d.ticketCards()
	} else {
		d.space(20)
		d.fill(cSurface)
		d.draw(cBorder)
		top := p.GetY()
		p.RoundedRect(margin, top, content, 16, 2.5, "1234", "FD")
		d.text(cMuted)
		d.font("", 9.5)
		p.SetXY(margin, top+5.5)
		d.cell(content, 5, "Belum ada tiket yang bisa dinilai pada periode ini.", "C", false)
		p.SetY(top + 22)
	}
	d.excludedAndNotes()
	if err := p.Error(); err != nil {
		return err
	}
	return p.Output(w)
}
