package falconcis

import "testing"

func TestFailedRulesFilter(t *testing.T) {
	t.Parallel()

	t.Run("empty framework filter", func(t *testing.T) {
		t.Parallel()

		if got := failedRulesFilter(""); got != "" {
			t.Fatalf("failedRulesFilter(\"\") = %q, want empty string", got)
		}
	})

	t.Run("framework filter without wildcard", func(t *testing.T) {
		t.Parallel()

		if got := failedRulesFilter("CIS"); got != "framework_name:'CIS'" {
			t.Fatalf("failedRulesFilter(\"CIS\") = %q, want %q", got, "framework_name:'CIS'")
		}
	})
}
