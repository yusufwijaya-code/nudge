package onboarding

import (
	"strings"

	"nudge/modules/tracking_rule"
)

// Defaults is the suggested form of a new board (the same names as tracking_rule.RuleInput).
type Defaults struct {
	TrackedStatuses             []string `json:"trackedStatuses"`
	DoneStatus                  string   `json:"doneStatus"`
	ReviewStatus                string   `json:"reviewStatus"`
	StoryPointFields            []string `json:"storyPointFields"`
	StoryPointWriteField        string   `json:"storyPointWriteField"`
	StoryPointWriteFieldSubtask string   `json:"storyPointWriteFieldSubtask"`
}

func fold(list []string, v string) (string, bool) {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return x, true
		}
	}
	return "", false
}

// Suggest copies the existing rule whose workflow fits the new project best
// (most tracked statuses the project also has), so a team on the same workflow
// saves in one click. Story Point fields are the template's that exist as number
// fields in the project, else the fields that look like Story Points. Nothing is
// suggested when no rule fits; the user then picks everything.
func Suggest(l tracking_rule.Lookup, rules []tracking_rule.RuleView) (Defaults, string) {
	var def Defaults
	var best *tracking_rule.RuleView
	bestScore := 0
	for i := range rules {
		r := &rules[i]
		if !r.IsActive {
			continue
		}
		score := 0
		for _, t := range r.TrackedStatuses {
			if _, ok := fold(l.Statuses, t); ok {
				score++
			}
		}
		if _, ok := fold(l.Statuses, r.DoneStatus); ok && score > 0 {
			score++
		}
		if score > bestScore {
			best, bestScore = r, score
		}
	}

	fields := map[string]bool{}
	for _, f := range l.NumberFields {
		fields[f.ID] = true
	}
	if best != nil {
		for _, t := range best.TrackedStatuses {
			if s, ok := fold(l.Statuses, t); ok {
				def.TrackedStatuses = append(def.TrackedStatuses, s)
			}
		}
		if d, ok := fold(l.Statuses, best.DoneStatus); ok {
			if _, tracked := fold(def.TrackedStatuses, d); !tracked {
				def.DoneStatus = d
			}
		}
		if best.ReviewStatus != nil {
			if rv, ok := fold(def.TrackedStatuses, *best.ReviewStatus); ok {
				def.ReviewStatus = rv
			}
		}
		for _, f := range best.StoryPointFields {
			if fields[f.ID] {
				def.StoryPointFields = append(def.StoryPointFields, f.ID)
			}
		}
	}
	if len(def.StoryPointFields) == 0 {
		for _, f := range l.NumberFields {
			if f.StoryPointLike {
				def.StoryPointFields = append(def.StoryPointFields, f.ID)
			}
		}
	}
	pick := func(want string) string {
		for _, f := range def.StoryPointFields {
			if f == want {
				return f
			}
		}
		if len(def.StoryPointFields) > 0 {
			return def.StoryPointFields[0]
		}
		return ""
	}
	if best != nil {
		def.StoryPointWriteField = pick(best.StoryPointWriteField)
		def.StoryPointWriteFieldSubtask = pick(best.StoryPointWriteFieldSubtask)
		return def, best.ProjectKey
	}
	def.StoryPointWriteField = pick("")
	def.StoryPointWriteFieldSubtask = pick("")
	return def, ""
}
