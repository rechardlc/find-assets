package main

import (
	"reflect"
	"testing"
)

func TestExpandShortFlags(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "period and pattern short",
			in:   []string{"scanner", "-p=day", "-pt=pierce"},
			want: []string{"scanner", "-period=day", "-pattern=pierce"},
		},
		{
			name: "space separated values",
			in:   []string{"scanner", "-p", "week", "-pt", "reversal"},
			want: []string{"scanner", "-period", "week", "-pattern", "reversal"},
		},
		{
			name: "serve and addr",
			in:   []string{"scanner", "-s", "-a", ":9090"},
			want: []string{"scanner", "-serve", "-addr", ":9090"},
		},
		{
			name: "source short",
			in:   []string{"scanner", "-so=em"},
			want: []string{"scanner", "-source=em"},
		},
		{
			name: "long flags unchanged",
			in:   []string{"scanner", "-period=day", "-pattern=pierce", "-workers=80"},
			want: []string{"scanner", "-period=day", "-pattern=pierce", "-workers=80"},
		},
		{
			name: "mixed short and long",
			in:   []string{"scanner", "-p=day", "-pattern=pierce", "-w", "50"},
			want: []string{"scanner", "-period=day", "-pattern=pierce", "-workers", "50"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := expandShortFlags(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("expandShortFlags(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
