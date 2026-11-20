package expr

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestArithmeticAndFunctions(t *testing.T) {
	cases := []struct {
		src  string
		want float64
	}{
		{"2 + 3 * 4", 14},
		{"(2 + 3) * 4", 20},
		{"2 ^ 3 ^ 2", 512},
		{"sin(pi/2) + cos(0)", 2},
		{"max(3, 7, 1) - min(4, 2)", 5},
		{"x = 5; x += 3; x * 2", 16},
		{"if (3 > 2) 42 else 0", 42},
		{"s = 0; for (i = 1; i <= 5; i++) s += i; s", 15},
		{"x = 1; y = 0; rotate(x, y, pi/2); round(y)", 1},
		{"a = 3; b = 9; swap(a, b); a", 9},
		{"(1 == 1 && 2 != 3) ? 10 : 20", 10},
		{"0x10 + 4", 20},
	}
	for _, tc := range cases {
		prog, err := Compile(tc.src)
		if err != nil {
			t.Fatalf("Compile(%q): %v", tc.src, err)
		}
		got, err := prog.Eval(NewEnv(time.Second))
		if err != nil {
			t.Fatalf("Eval(%q): %v", tc.src, err)
		}
		if math.Abs(got-tc.want) > 1e-6 {
			t.Fatalf("Eval(%q) = %v, want %v", tc.src, got, tc.want)
		}
	}
}

func TestInfiniteLoopTimeout(t *testing.T) {
	prog, err := Compile("while (1) { x = x + 1 }")
	if err != nil {
		t.Fatal(err)
	}
	env := NewEnv(50 * time.Millisecond)
	env.MaxSteps = 5000
	_, err = prog.Eval(env)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
}
