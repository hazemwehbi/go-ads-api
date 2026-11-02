//go:build integration && loadtest

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	adshttp "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/http"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/userclient"
)

type loadSample struct {
	Op            string
	Status        int
	MS            float64
	Invalid, Null bool
}
type loadSummary struct {
	Count              int
	P50, P95, P99, Max float64
}

// Opt-in, closed-loop local workload. Timing is measured without the race detector.
func TestLimitedLoad(t *testing.T) {
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer slog.SetDefault(old)
	for _, sc := range []struct {
		Name             string
		Workers          int
		SlowUser, SlowDB bool
	}{
		{"healthy", 20, false, false}, {"slow_users", 40, true, false}, {"slow_database_writes", 40, false, true}, {"healthy_again", 20, false, false},
	} {
		t.Run(sc.Name, func(t *testing.T) {
			d, _ := integrationDAO(t)
			_, err := d.db.Exec(`INSERT INTO ads(title,price_cents,photo_url,user_id) SELECT 'load bike '||i,i%10000,'https://example.com/a',md5((i%200)::text)::uuid FROM generate_series(1,10000) i`)
			require.NoError(t, err)
			_, err = d.db.Exec(`ANALYZE ads`)
			require.NoError(t, err)
			if sc.SlowDB {
				_, err = d.db.Exec(`CREATE FUNCTION delay_write() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(0.05); RETURN NEW; END $$; CREATE TRIGGER delay_write BEFORE INSERT ON ads FOR EACH ROW EXECUTE FUNCTION delay_write()`)
				require.NoError(t, err)
			}
			var calls, active, maximum atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				n := active.Add(1)
				defer active.Add(-1)
				for prev := maximum.Load(); n > prev; prev = maximum.Load() {
					if maximum.CompareAndSwap(prev, n) {
						break
					}
				}
				if sc.SlowUser {
					timer := time.NewTimer(500 * time.Millisecond)
					defer timer.Stop()
					select {
					case <-timer.C:
					case <-r.Context().Done():
						return
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]string{"user_id": strings.TrimPrefix(r.URL.Path, "/v1/users/"), "nickname": "load-user"})
			}))
			defer upstream.Close()
			lookup := userclient.NewResilient(userclient.New(upstream.URL, 250*time.Millisecond), userclient.DefaultOptions())
			server := httptest.NewServer(adshttp.NewRouter(adshttp.Usecases{GetAd: usecase.NewGetAdByIDFunc(d, lookup), ListAds: usecase.NewListAdsFunc(d), CreateAd: usecase.NewCreateAdFunc(d)}))
			defer server.Close()
			tr := &http.Transport{MaxIdleConns: 100, MaxIdleConnsPerHost: 100}
			defer tr.CloseIdleConnections()
			client := &http.Client{Transport: tr, Timeout: 7 * time.Second}
			jobs := make(chan int)
			samples := make(chan loadSample, 3000)
			var wg sync.WaitGroup
			before := d.db.Stats()
			start := time.Now()
			for worker := 0; worker < sc.Workers; worker++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for i := range jobs {
						op, path, method, body := "detail", fmt.Sprintf("/ads/%d", 1+(i*7919)%10000), "GET", ""
						if i%10 == 1 {
							op, path = "list", "/ads?title=bike&min_price=10&limit=20"
						}
						if i%10 == 0 {
							op, path, method, body = "create", "/ads", "POST", `{"title":"load-created","price":12.34,"photo_url":"https://example.com/a"}`
						}
						req, e := http.NewRequestWithContext(context.Background(), method, server.URL+path, strings.NewReader(body))
						if e != nil {
							samples <- loadSample{Op: op, Invalid: true}
							continue
						}
						if method == "POST" {
							req.Header.Set("Content-Type", "application/json")
							req.Header.Set("Authorization", fmt.Sprintf("dddddddd-dddd-4ddd-addd-%012d", (i/10)%20+1))
							req.Header.Set("Idempotency-Key", fmt.Sprintf("load-%d", i))
						}
						began := time.Now()
						resp, e := client.Do(req)
						sample := loadSample{Op: op}
						if e != nil {
							sample.Invalid = true
						} else {
							sample.Status = resp.StatusCode
							raw, e := io.ReadAll(resp.Body)
							closeErr := resp.Body.Close()
							var obj map[string]json.RawMessage
							sample.Invalid = closeErr != nil || e != nil || json.Unmarshal(raw, &obj) != nil
							if !sample.Invalid {
								switch op {
								case "detail":
									name, exists := obj["username"]
									sample.Null = string(name) == "null"
									sample.Invalid = resp.StatusCode != 200 || !exists || string(obj["id"]) != fmt.Sprint(1+(i*7919)%10000) || (!sample.Null && string(name) != `"load-user"`)
								case "list":
									var ads []json.RawMessage
									sample.Invalid = resp.StatusCode != 200 || json.Unmarshal(obj["ads"], &ads) != nil || len(ads) != 20
								case "create":
									sample.Invalid = (resp.StatusCode != 201 && resp.StatusCode != 429) || (resp.StatusCode == 201 && len(obj["id"]) == 0) || (resp.StatusCode == 429 && len(obj["error"]) == 0)
								}
							}
						}
						sample.MS = float64(time.Since(began).Microseconds()) / 1000
						samples <- sample
					}
				}()
			}
			for i := 0; i < 3000; i++ {
				jobs <- i
			}
			close(jobs)
			wg.Wait()
			close(samples)
			elapsed := time.Since(start)
			after := d.db.Stats()
			statuses := map[int]int{}
			times := map[string][]float64{}
			invalid, nulls := 0, 0
			for s := range samples {
				statuses[s.Status]++
				key := fmt.Sprintf("%s_%d", s.Op, s.Status)
				times[key] = append(times[key], s.MS)
				if s.Invalid {
					invalid++
				}
				if s.Null {
					nulls++
				}
			}
			timings := map[string]loadSummary{}
			for op, v := range times {
				sort.Float64s(v)
				pct := func(p int) float64 { return v[(len(v)*p+99)/100-1] }
				timings[op] = loadSummary{len(v), pct(50), pct(95), pct(99), v[len(v)-1]}
			}
			counts := map[string]int{}
			for _, table := range []string{"ads", "ad_publications", "ad_imports"} {
				var n int
				require.NoError(t, d.db.QueryRow("SELECT count(*) FROM "+table).Scan(&n))
				counts[table] = n
			}
			var quotaMax int
			require.NoError(t, d.db.QueryRow(`SELECT coalesce(max(n),0) FROM (SELECT count(*) n FROM ad_publications GROUP BY owner_id) q`).Scan(&quotaMax))
			result := map[string]any{"scenario": sc.Name, "workers": sc.Workers, "requests": 3000, "seconds": elapsed.Seconds(), "rps": 3000 / elapsed.Seconds(), "statuses": statuses, "timings": timings, "invalid": invalid, "null_usernames": nulls, "upstream_calls": calls.Load(), "upstream_max_active": maximum.Load(), "db_wait_count": after.WaitCount - before.WaitCount, "db_wait_total_ms": float64((after.WaitDuration - before.WaitDuration).Microseconds()) / 1000, "counts": counts, "max_publications_per_owner": quotaMax}
			raw, e := json.Marshal(result)
			require.NoError(t, e)
			t.Log("LOAD_RESULT " + string(raw))
			require.Zero(t, invalid)
			require.Equal(t, 2700, statuses[200])
			require.Equal(t, 100, statuses[201])
			require.Equal(t, 200, statuses[429])
			require.Equal(t, 10100, counts["ads"])
			require.Equal(t, 100, counts["ad_publications"])
			require.Equal(t, 100, counts["ad_imports"])
			require.Equal(t, 5, quotaMax)
			require.LessOrEqual(t, maximum.Load(), int64(40))
			if sc.SlowUser {
				require.Equal(t, 2400, nulls)
			}
		})
	}
}
