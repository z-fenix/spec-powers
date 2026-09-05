// Package cronexpr parses five-field cron expressions and computes their
// next fire times. It is the trigger parser behind cron-driven autopilots.
package cronexpr

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// ErrInvalidSpec is returned for a malformed cron expression.
var ErrInvalidSpec = errors.New("invalid cron spec")

// Schedule is a parsed five-field cron expression: minute hour day-of-month
// month day-of-week. Fields support "*", lists ("a,b"), ranges ("a-b") and
// steps ("*/n", "a-b/n"). Next works in the location of the time it is given.
type Schedule struct {
	minute bitset
	hour   bitset
	dom    bitset // index = day-1
	month  bitset // index = month-1
	dow    bitset // 0 = Sunday
	// domStar / dowStar record whether the field was "*"; when both are
	// restricted, a day matches if EITHER field matches (standard cron
	// semantics).
	domStar bool
	dowStar bool
}

// Parse parses a five-field cron expression.
func Parse(spec string) (Schedule, error) {
	var s Schedule
	fields := strings.Fields(spec)
	if len(fields) != 5 {
		return s, ErrInvalidSpec
	}
	var err error
	if s.minute, err = parseField(fields[0], 0, 59); err != nil {
		return s, err
	}
	if s.hour, err = parseField(fields[1], 0, 23); err != nil {
		return s, err
	}
	if s.dom, err = parseField(fields[2], 1, 31); err != nil {
		return s, err
	}
	if s.month, err = parseField(fields[3], 1, 12); err != nil {
		return s, err
	}
	if s.dow, err = parseField(fields[4], 0, 6); err != nil {
		return s, err
	}
	s.domStar = fields[2] == "*"
	s.dowStar = fields[4] == "*"
	return s, nil
}

// Next returns the first minute strictly after `after` that matches the
// schedule, or the zero time when no match exists within the next five
// years (e.g. February 30th).
func (s Schedule) Next(after time.Time) time.Time {
	t := after.Truncate(time.Minute).Add(time.Minute)
	limit := after.AddDate(5, 0, 0)
	for t.Before(limit) {
		if !s.month[t.Month()-1] {
			t = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location()).AddDate(0, 1, 0)
			continue
		}
		if !s.dayMatches(t) {
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).AddDate(0, 0, 1)
			continue
		}
		if !s.hour[t.Hour()] {
			// first matching hour later today, else tomorrow midnight
			next := -1
			for h := t.Hour() + 1; h < 24; h++ {
				if s.hour[h] {
					next = h
					break
				}
			}
			if next < 0 {
				t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).AddDate(0, 0, 1)
			} else {
				t = time.Date(t.Year(), t.Month(), t.Day(), next, 0, 0, 0, t.Location())
			}
			continue
		}
		if s.minute[t.Minute()] {
			return t
		}
		next := -1
		for m := t.Minute() + 1; m < 60; m++ {
			if s.minute[m] {
				next = m
				break
			}
		}
		if next < 0 {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location()).Add(time.Hour)
		} else {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), next, 0, 0, t.Location())
		}
	}
	return time.Time{}
}

// dayMatches applies cron's dom/dow rule: "*" means unrestricted; when both
// fields are restricted the day matches if either one does.
func (s Schedule) dayMatches(t time.Time) bool {
	domOK := s.domStar || s.dom[t.Day()-1]
	dowOK := s.dowStar || s.dow[int(t.Weekday())]
	switch {
	case s.domStar && s.dowStar:
		return true
	case s.domStar:
		return dowOK
	case s.dowStar:
		return domOK
	default:
		return domOK || dowOK
	}
}

// parseField expands one cron field into a bitset over [min, max].
func parseField(field string, min, max int) (bitset, error) {
	out := newBitset(max - min + 1)
	for _, part := range strings.Split(field, ",") {
		rangePart := part
		step := 1
		if i := strings.Index(part, "/"); i >= 0 {
			rangePart = part[:i]
			parsed, err := strconv.Atoi(part[i+1:])
			if err != nil || parsed <= 0 {
				return out, ErrInvalidSpec
			}
			step = parsed
		}
		if err := setRange(out, rangePart, min, max, step); err != nil {
			return out, err
		}
	}
	for _, v := range out {
		if v {
			return out, nil
		}
	}
	return out, ErrInvalidSpec
}

type bitset []bool

func newBitset(n int) bitset { return make(bitset, n) }

// setRange marks the values selected by one comma-separated part: "*",
// "a", "a-b", each optionally followed by "/step". A bare "*/n" starts at
// the field minimum.
func setRange(out bitset, part string, min, max, step int) error {
	lo, hi := min, max
	if part != "*" {
		dash := strings.Index(part, "-")
		switch {
		case dash < 0:
			v, err := strconv.Atoi(part)
			if err != nil {
				return ErrInvalidSpec
			}
			lo, hi = v, v
		case dash == 0 || dash == len(part)-1:
			return ErrInvalidSpec
		default:
			var err error
			if lo, err = strconv.Atoi(part[:dash]); err != nil {
				return ErrInvalidSpec
			}
			if hi, err = strconv.Atoi(part[dash+1:]); err != nil {
				return ErrInvalidSpec
			}
		}
	}
	if lo < min || hi > max || lo > hi {
		return ErrInvalidSpec
	}
	for v := lo; v <= hi; v += step {
		out[v-min] = true
	}
	return nil
}
