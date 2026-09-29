// Package allowlist — известные нарушения новой проверки гейта: гейт зелёный, а долг виден и
// конечен.
package allowlist

import (
	"regexp"
	"slices"
	"testing"
)

// List — нарушение → ID находки аудита, которая его закрывает.
type List map[string]string

var findingID = regexp.MustCompile(`^[A-Z]{2,3}-\d{2}$`)

// Check: нарушение вне списка — провал; запись без ID находки или без нарушения — тоже:
// исправленное вычёркивается.
func (l List) Check(t testing.TB, violations []string, rule string) {
	t.Helper()
	for _, v := range violations {
		if _, known := l[v]; !known {
			t.Errorf("%s: %s", v, rule)
		}
	}
	for v, id := range l {
		switch {
		case !findingID.MatchString(id):
			t.Errorf("allowlist entry %q needs an audit finding ID such as ADP-02, got %q", v, id)
		case !slices.Contains(violations, v):
			t.Errorf("allowlist entry %q (%s) matches no violation: remove it", v, id)
		}
	}
}
