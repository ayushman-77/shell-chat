package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// DefaultLocation returns the default timezone (IST / UTC+5:30) for Indian users, or system local.
func DefaultLocation() *time.Location {
	loc := time.Local
	_, offset := time.Now().In(loc).Zone()
	// If the host is a standard cloud VM running in UTC/GMT (offset 0), default to IST (Asia/Kolkata)
	if offset == 0 {
		if ist, err := time.LoadLocation("Asia/Kolkata"); err == nil {
			return ist
		}
		return time.FixedZone("IST", 5*3600+30*60)
	}
	return loc
}

// ParseTimezone parses a timezone name, abbreviation, or UTC offset into a *time.Location.
func ParseTimezone(input string) (*time.Location, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return DefaultLocation(), nil
	}

	upper := strings.ToUpper(input)
	switch upper {
	case "IST", "INDIA":
		return time.FixedZone("IST", 5*3600+30*60), nil
	case "UTC", "GMT", "Z":
		return time.UTC, nil
	case "EST":
		return time.FixedZone("EST", -5*3600), nil
	case "EDT":
		return time.FixedZone("EDT", -4*3600), nil
	case "CST":
		return time.FixedZone("CST", -6*3600), nil
	case "CDT":
		return time.FixedZone("CDT", -5*3600), nil
	case "MST":
		return time.FixedZone("MST", -7*3600), nil
	case "MDT":
		return time.FixedZone("MDT", -6*3600), nil
	case "PST":
		return time.FixedZone("PST", -8*3600), nil
	case "PDT":
		return time.FixedZone("PDT", -7*3600), nil
	case "BST":
		return time.FixedZone("BST", 1*3600), nil
	case "CET":
		return time.FixedZone("CET", 1*3600), nil
	case "CEST":
		return time.FixedZone("CEST", 2*3600), nil
	case "JST", "TOKYO", "JAPAN":
		return time.FixedZone("JST", 9*3600), nil
	case "KST", "SEOUL", "KOREA":
		return time.FixedZone("KST", 9*3600), nil
	case "SGT", "SINGAPORE":
		return time.FixedZone("SGT", 8*3600), nil
	case "AEST":
		return time.FixedZone("AEST", 10*3600), nil
	case "AEDT":
		return time.FixedZone("AEDT", 11*3600), nil
	case "NZST":
		return time.FixedZone("NZST", 12*3600), nil
	}

	// Try standard IANA database location (e.g. Asia/Kolkata, America/New_York)
	if loc, err := time.LoadLocation(input); err == nil {
		return loc, nil
	}

	// Try parsing numeric offset like "+5:30", "+05:30", "+5.5", "-4", "+8"
	clean := strings.TrimPrefix(upper, "UTC")
	clean = strings.TrimPrefix(clean, "GMT")
	clean = strings.TrimSpace(clean)

	if clean != "" {
		sign := 1
		if strings.HasPrefix(clean, "-") {
			sign = -1
			clean = clean[1:]
		} else if strings.HasPrefix(clean, "+") {
			clean = clean[1:]
		}

		if strings.Contains(clean, ":") {
			parts := strings.Split(clean, ":")
			hours, err1 := strconv.Atoi(parts[0])
			mins, err2 := strconv.Atoi(parts[1])
			if err1 == nil && err2 == nil {
				sec := sign * (hours*3600 + mins*60)
				name := fmt.Sprintf("UTC%+03d:%02d", sign*hours, mins)
				return time.FixedZone(name, sec), nil
			}
		} else if strings.Contains(clean, ".") {
			if val, err := strconv.ParseFloat(clean, 64); err == nil {
				sec := int(float64(sign) * val * 3600)
				name := fmt.Sprintf("UTC%+0.1f", float64(sign)*val)
				return time.FixedZone(name, sec), nil
			}
		} else {
			if hours, err := strconv.Atoi(clean); err == nil {
				sec := sign * hours * 3600
				name := fmt.Sprintf("UTC%+03d:00", sign*hours)
				return time.FixedZone(name, sec), nil
			}
		}
	}

	return nil, fmt.Errorf("unknown timezone %q", input)
}
