package horisen

import "testing"

func TestIsSuccess(t *testing.T) {
	cases := map[Code]bool{
		100: true,
		101: true,
		102: false,
		105: false,
		116: false,
	}
	for code, want := range cases {
		if got := IsSuccess(code); got != want {
			t.Errorf("IsSuccess(%d) = %v, want %v", code, got, want)
		}
	}
}

func TestIsRetryable(t *testing.T) {
	// Only 105 (throttled) should be retryable at the application layer.
	cases := map[Code]bool{
		100: false, // OK — not an error
		102: false,
		103: false,
		104: false,
		105: true,
		106: false,
		107: false,
		108: false,
		109: false,
		110: false,
		111: false,
		112: false,
		113: false,
		114: false,
		115: false,
		116: false,
	}
	for code, want := range cases {
		if got := IsRetryable(code); got != want {
			t.Errorf("IsRetryable(%d) = %v, want %v", code, got, want)
		}
	}
}

func TestError_ErrorMessage(t *testing.T) {
	e := &Error{Code: 103, Description: "invalid receiver"}
	want := "horisen: code=103 invalid receiver"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
