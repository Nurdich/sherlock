//go:build online

package search

import (
	"context"
	"testing"
	"time"

	"github.com/Nurdich/sherlock/internal/result"
	"github.com/Nurdich/sherlock/internal/sites"
)

func TestLiveKnownAccounts(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	list, err := sites.LoadLocal()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		site string
		user string
		want result.Status
	}{
		{"GitHub", "ppfeister", result.Claimed},
		{"GitLab", "ppfeister", result.Claimed},
	}
	for _, tc := range cases {
		site, ok := sites.ByName(list, tc.site)
		if !ok {
			t.Fatalf("missing %s", tc.site)
		}
		hits, err := Run(context.Background(), tc.user, []sites.Site{site}, nil, Config{Timeout: 30 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		if hits[0].Result.Status != tc.want {
			t.Fatalf("%s %s = %s (%v)", tc.site, tc.user, hits[0].Result.Status, hits[0].Result.Context)
		}
	}
}
