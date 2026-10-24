package i18n

import "testing"

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		k, v string
		ok   bool
	}{
		{"confirm.subject", "Bevestig %s", true},
		{"confirm.subject", "Bevestig", false},
		{"confirm.subject", "%d", false},
		{"confirm.subject", "%[1]s", false},
		{"footer.since", "%s op %s", true},
		{"form.submit", "100%% klaar", true},
		{"form.submit", "100% klaar", false},
		{"unknown", "", false},
		{"date", "2006-01-02", true},
		{"date", "02 January 2006", true},
		{"date", "Monday", false},
		{"date", "2006--02", false},
		{"confirm.subject", "", true},
	} {
		if err := Validate(tc.k, tc.v); (err == nil) != tc.ok {
			t.Errorf("%s %q: %v", tc.k, tc.v, err)
		}
	}
}

func FuzzValidate(f *testing.F) {
	f.Add("confirm.subject", "Bevestig %s")
	f.Add("date", "2006-01-02")
	f.Fuzz(func(t *testing.T, k, v string) { _ = Validate(k, v) })
}
