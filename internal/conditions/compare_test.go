package conditions

import "testing"

func TestEvaluate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		actual   string
		operator string
		expected string
		want     bool
		wantErr  string
	}{
		{
			name:     "equal true",
			actual:   "ok",
			operator: "=",
			expected: "ok",
			want:     true,
		},
		{
			name:     "equal false",
			actual:   "ok",
			operator: "=",
			expected: "nope",
			want:     false,
		},
		{
			name:     "not equal true",
			actual:   "ok",
			operator: "!=",
			expected: "nope",
			want:     true,
		},
		{
			name:     "greater than",
			actual:   "10",
			operator: ">",
			expected: "2",
			want:     true,
		},
		{
			name:     "less than",
			actual:   "2",
			operator: "<",
			expected: "10",
			want:     true,
		},
		{
			name:     "greater or equal with spaces",
			actual:   " 10 ",
			operator: ">=",
			expected: "10",
			want:     true,
		},
		{
			name:     "less or equal false",
			actual:   "11",
			operator: "<=",
			expected: "10",
			want:     false,
		},
		{
			name:     "contains case insensitive",
			actual:   "Alice Goblin",
			operator: "contains",
			expected: "goblin",
			want:     true,
		},
		{
			name:     "like alias",
			actual:   "Alice Goblin",
			operator: "like",
			expected: "alice",
			want:     true,
		},
		{
			name:     "operator is trimmed and lowercased",
			actual:   "Alice Goblin",
			operator: "  CONTAINS ",
			expected: "gob",
			want:     true,
		},
		{
			name:     "unsupported operator",
			actual:   "1",
			operator: "??",
			expected: "1",
			wantErr:  "unsupported operator: ??",
		},
		{
			name:     "actual is not numeric",
			actual:   "abc",
			operator: ">",
			expected: "1",
			wantErr:  "actual value is not numeric",
		},
		{
			name:     "expected is not numeric",
			actual:   "1",
			operator: ">",
			expected: "abc",
			wantErr:  "expected value is not numeric",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Evaluate(tt.actual, tt.operator, tt.expected)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Evaluate() error = nil, want %q", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("Evaluate() error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("Evaluate() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("Evaluate() = %v, want %v", got, tt.want)
			}
		})
	}
}
