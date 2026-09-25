package providers

import (
	"testing"
)

func TestPermanentError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		err         error
		isPermanent bool
		isTransient bool
	}{
		{
			name:        "permanent error is classified correctly",
			err:         PermanentError(ErrInvalidRecipient),
			isPermanent: true,
			isTransient: false,
		},
		{
			name:        "transient error is classified correctly",
			err:         TransientError(ErrGatewayTimeout),
			isPermanent: false,
			isTransient: true,
		},
		{
			name:        "plain error is neither",
			err:         ErrInvalidRecipient,
			isPermanent: false,
			isTransient: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := IsPermanent(tt.err); got != tt.isPermanent {
				t.Errorf("IsPermanent() = %v, want %v", got, tt.isPermanent)
			}
			if got := IsTransient(tt.err); got != tt.isTransient {
				t.Errorf("IsTransient() = %v, want %v", got, tt.isTransient)
			}
		})
	}
}
