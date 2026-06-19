package tdx

import (
	"testing"
	"time"
)

func TestUpdatedLatestNodeSupportsMultipleDailyCheckpoints(t *testing.T) {
	u := &Updated{
		nodes: []updateNode{
			{hour: 9, minute: 0},
			{hour: 15, minute: 0},
		},
	}

	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "before first checkpoint uses previous day last checkpoint",
			now:  time.Date(2026, 6, 19, 8, 59, 0, 0, time.Local),
			want: time.Date(2026, 6, 18, 15, 0, 0, 0, time.Local),
		},
		{
			name: "after morning checkpoint",
			now:  time.Date(2026, 6, 19, 9, 1, 0, 0, time.Local),
			want: time.Date(2026, 6, 19, 9, 0, 0, 0, time.Local),
		},
		{
			name: "after afternoon checkpoint",
			now:  time.Date(2026, 6, 19, 15, 1, 0, 0, time.Local),
			want: time.Date(2026, 6, 19, 15, 0, 0, 0, time.Local),
		},
		{
			name: "next morning before first checkpoint",
			now:  time.Date(2026, 6, 20, 8, 59, 0, 0, time.Local),
			want: time.Date(2026, 6, 19, 15, 0, 0, 0, time.Local),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := u.latestNode(tc.now); !got.Equal(tc.want) {
				t.Fatalf("latestNode(%s)=%s, want %s", tc.now, got, tc.want)
			}
		})
	}
}
