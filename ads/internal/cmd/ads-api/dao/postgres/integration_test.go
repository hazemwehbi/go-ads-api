//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	adsdao "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/dao"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
)

const testOwner domain.UserID = "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa"
const otherOwner domain.UserID = "bbbbbbbb-bbbb-4bbb-bbbb-bbbbbbbbbbbb"

// Each test gets a separate schema. Only explicitly named *_test databases are accepted.
func integrationDAO(t *testing.T) (*DAO, string) {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	u, err := url.Parse(raw)
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(u.Path, "_test"), "TEST_DATABASE_URL must name a dedicated *_test database")
	admin, err := sql.Open("pgx", raw)
	require.NoError(t, err)
	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = admin.Exec(`CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public`)
	require.NoError(t, err)
	_, err = admin.Exec(`CREATE SCHEMA ` + schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, e := admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
		if e != nil {
			t.Errorf("cleanup: %v", e)
		}
		_ = admin.Close()
	})
	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()
	d, err := New(context.Background(), u.String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })
	files, err := filepath.Glob("migrations/*.sql")
	require.NoError(t, err)
	for _, f := range files {
		if strings.Contains(filepath.Base(f), "002") {
			continue
		}
		b, e := os.ReadFile(f)
		require.NoError(t, e)
		_, e = d.db.Exec(string(b))
		require.NoError(t, e, f)
	}
	return d, u.String()
}

func adRequest(owner domain.UserID) usecase.CreateAdRequest {
	return usecase.CreateAdRequest{OwnerID: owner, Title: "bike", PriceCents: 1999, PhotoURL: "https://example.com/bike.jpg"}
}
func importRequest(owner domain.UserID, key string) usecase.ImportAdsRequest {
	return usecase.ImportAdsRequest{OwnerID: owner, IdempotencyKey: key, Ads: []usecase.ImportedAd{
		{Title: "bike", PriceCents: 1999, PhotoURL: "https://example.com/bike.jpg"},
		{Title: "scooter", PriceCents: 5000, PhotoURL: "https://example.com/scooter.jpg"},
	}}
}

func TestIntegrationQuotaConcurrentInstances(t *testing.T) {
	d, dsn := integrationDAO(t)
	d2, err := New(context.Background(), dsn)
	require.NoError(t, err)
	defer d2.Close()
	funcs := []usecase.CreateAdFunc{usecase.NewCreateAdFunc(d), usecase.NewCreateAdFunc(d2)}
	type result struct {
		id  domain.AdID
		err error
	}
	out := make(chan result, 30)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			owner := testOwner
			if i%2 == 0 {
				owner = domain.UserID(strings.ToUpper(string(owner)))
			}
			id, e := funcs[i%2](context.Background(), adRequest(owner))
			out <- result{id, e}
		}(i)
	}
	close(start)
	wg.Wait()
	close(out)
	ids := []domain.AdID{}
	denied := 0
	for r := range out {
		if r.err == nil {
			ids = append(ids, r.id)
		} else {
			require.ErrorIs(t, r.err, usecase.ErrQuotaExceeded)
			denied++
		}
	}
	require.Len(t, ids, 5)
	require.Equal(t, 25, denied)
	// Deleting an ad does not refund a publication. Another owner has a separate quota.
	require.NoError(t, usecase.NewDeleteAdFunc(d)(context.Background(), usecase.DeleteAdRequest{ID: ids[0], OwnerID: testOwner}))
	_, err = funcs[0](context.Background(), adRequest(testOwner))
	require.ErrorIs(t, err, usecase.ErrQuotaExceeded)
	_, err = funcs[1](context.Background(), adRequest(otherOwner))
	require.NoError(t, err)
	// Import remains possible at the quota and must not consume or record publications.
	_, err = usecase.NewImportAdsFunc(d)(context.Background(), importRequest(testOwner, "at-quota"))
	require.NoError(t, err)
	var n int
	require.NoError(t, d.db.QueryRow("SELECT count(*) FROM ad_publications WHERE owner_id=$1", string(testOwner)).Scan(&n))
	require.Equal(t, 5, n)
	_, err = d.db.Exec("UPDATE ad_publications SET published_at=clock_timestamp()-interval '25 hours' WHERE owner_id=$1", string(testOwner))
	require.NoError(t, err)
	for i := 0; i < 5; i++ {
		_, err = funcs[i%2](context.Background(), adRequest(testOwner))
		require.NoError(t, err)
	}
	_, err = funcs[0](context.Background(), adRequest(testOwner))
	require.ErrorIs(t, err, usecase.ErrQuotaExceeded)
}

func TestIntegrationImportConcurrentReplayAndOwnership(t *testing.T) {
	d, dsn := integrationDAO(t)
	d2, err := New(context.Background(), dsn)
	require.NoError(t, err)
	defer d2.Close()
	funcs := []usecase.ImportAdsFunc{usecase.NewImportAdsFunc(d), usecase.NewImportAdsFunc(d2)}
	type result struct {
		ids []domain.AdID
		err error
	}
	out := make(chan result, 20)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			ids, e := funcs[i%2](context.Background(), importRequest(testOwner, "same-key"))
			out <- result{ids, e}
		}(i)
	}
	close(start)
	wg.Wait()
	close(out)
	var want []domain.AdID
	for r := range out {
		require.NoError(t, r.err)
		if want == nil {
			want = r.ids
		}
		require.Equal(t, want, r.ids)
	}
	require.Len(t, want, 2)
	var count int
	require.NoError(t, d.db.QueryRow("SELECT count(*) FROM ads").Scan(&count))
	require.Equal(t, 2, count)
	req := importRequest(testOwner, "same-key")
	req.Ads[0].PriceCents++
	_, err = funcs[0](context.Background(), req)
	require.ErrorIs(t, err, usecase.ErrIdempotencyKeyReused)
	ids, err := funcs[1](context.Background(), importRequest(otherOwner, "same-key"))
	require.NoError(t, err)
	require.NotEqual(t, want, ids)
	// Imports do not use any of the five POST /ads slots.
	for i := 0; i < 5; i++ {
		_, err = usecase.NewCreateAdFunc(d)(context.Background(), adRequest(otherOwner))
		require.NoError(t, err)
	}
	// A replay remains a receipt for the original import, even after a subsequent deletion.
	err = usecase.NewDeleteAdFunc(d)(context.Background(), usecase.DeleteAdRequest{ID: want[0], OwnerID: testOwner})
	require.NoError(t, err)
	again, err := funcs[0](context.Background(), importRequest(testOwner, "same-key"))
	require.NoError(t, err)
	require.Equal(t, want, again)
}

func TestIntegrationImportRollbackAndRetry(t *testing.T) {
	d, _ := integrationDAO(t)
	_, err := d.db.Exec(`CREATE FUNCTION fail_second() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.title='scooter' THEN RAISE EXCEPTION 'injected failure'; END IF; RETURN NEW; END $$;
 CREATE TRIGGER fail_second BEFORE INSERT ON ads FOR EACH ROW EXECUTE FUNCTION fail_second()`)
	require.NoError(t, err)
	fn := usecase.NewImportAdsFunc(d)
	req := importRequest(testOwner, "retry-after-failure")
	_, err = fn(context.Background(), req)
	require.Error(t, err)
	var ads, keys int
	require.NoError(t, d.db.QueryRow("SELECT count(*) FROM ads").Scan(&ads))
	require.NoError(t, d.db.QueryRow("SELECT count(*) FROM ad_imports").Scan(&keys))
	require.Zero(t, ads)
	require.Zero(t, keys)
	_, err = d.db.Exec("DROP TRIGGER fail_second ON ads")
	require.NoError(t, err)
	ids, err := fn(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, ids, 2)
	replay, err := fn(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, ids, replay)
}

func TestIntegrationCancellationReleasesTransactions(t *testing.T) {
	d, _ := integrationDAO(t)
	ctx := context.Background()
	tx, err := d.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback()
	require.NoError(t, tx.LockOwnerPublications(ctx, testOwner))
	blocked, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	_, err = usecase.NewCreateAdFunc(d)(blocked, adRequest(testOwner))
	require.Error(t, err)
	require.NoError(t, tx.Rollback())
	_, err = usecase.NewCreateAdFunc(d)(ctx, adRequest(testOwner))
	require.NoError(t, err)
	tx, err = d.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback()
	_, err = tx.ClaimRequest(ctx, adsdao.RequestKey{OwnerID: testOwner, IdempotencyKey: "waiting"}, "uncommitted")
	require.NoError(t, err)
	blocked2, cancel2 := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel2()
	_, err = usecase.NewImportAdsFunc(d)(blocked2, importRequest(testOwner, "waiting"))
	require.Error(t, err)
	require.NoError(t, tx.Rollback())
	_, err = usecase.NewImportAdsFunc(d)(ctx, importRequest(testOwner, "waiting"))
	require.NoError(t, err)
}

func TestIntegrationDeleteAndSearch(t *testing.T) {
	d, _ := integrationDAO(t)
	ctx := context.Background()
	fn := usecase.NewCreateAdFunc(d)
	req := adRequest(testOwner)
	req.Title = "100%_bike"
	id, err := fn(ctx, req)
	require.NoError(t, err)
	_, err = fn(ctx, adRequest(testOwner))
	require.NoError(t, err)
	del := usecase.NewDeleteAdFunc(d)
	require.ErrorIs(t, del(ctx, usecase.DeleteAdRequest{ID: id, OwnerID: otherOwner}), adsdao.ErrNotFound)
	ads, err := d.SearchAds(ctx, adsdao.SearchAdsRequest{Title: "%_", Limit: 1})
	require.NoError(t, err)
	require.Len(t, ads, 1)
	require.Equal(t, id, ads[0].ID)
	ads, err = d.SearchAds(ctx, adsdao.SearchAdsRequest{OwnerID: testOwner, Limit: 1})
	require.NoError(t, err)
	require.Len(t, ads, 2)
	next, err := d.SearchAds(ctx, adsdao.SearchAdsRequest{OwnerID: testOwner, Limit: 1, BeforeID: ads[0].ID})
	require.NoError(t, err)
	require.Len(t, next, 1)
	require.Equal(t, id, next[0].ID)
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { results <- del(ctx, usecase.DeleteAdRequest{ID: id, OwnerID: testOwner}) }()
	}
	a, b := <-results, <-results
	require.True(t, (a == nil && errors.Is(b, adsdao.ErrNotFound)) || (b == nil && errors.Is(a, adsdao.ErrNotFound)))
	_, err = d.GetAdByID(ctx, id)
	require.ErrorIs(t, err, adsdao.ErrNotFound)
}

func TestIntegrationPublicationFailureRollsBackAd(t *testing.T) {
	d, _ := integrationDAO(t)
	_, err := d.db.Exec(`CREATE FUNCTION fail_publication() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected publication failure'; END $$;
 CREATE TRIGGER fail_publication BEFORE INSERT ON ad_publications FOR EACH ROW EXECUTE FUNCTION fail_publication()`)
	require.NoError(t, err)
	_, err = usecase.NewCreateAdFunc(d)(context.Background(), adRequest(testOwner))
	require.Error(t, err)
	var ads, events int
	require.NoError(t, d.db.QueryRow("SELECT count(*) FROM ads").Scan(&ads))
	require.NoError(t, d.db.QueryRow("SELECT count(*) FROM ad_publications").Scan(&events))
	require.Zero(t, ads)
	require.Zero(t, events)
	_, err = d.db.Exec("DROP TRIGGER fail_publication ON ad_publications")
	require.NoError(t, err)
	_, err = usecase.NewCreateAdFunc(d)(context.Background(), adRequest(testOwner))
	require.NoError(t, err)
}
func TestIntegrationRollingBoundaryAndZeroPrice(t *testing.T) {
	d, _ := integrationDAO(t)
	ctx := context.Background()
	// One event is expired, another is still inside the rolling window.
	_, err := d.db.Exec(`INSERT INTO ad_publications(ad_id,owner_id,published_at) VALUES
 (-1,$1,statement_timestamp()-interval '24 hours 1 second'),
 (-2,$1,statement_timestamp()-interval '23 hours 59 minutes')`, string(testOwner))
	require.NoError(t, err)
	n, err := d.CountRecentPublications(ctx, testOwner, 24*time.Hour)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	req := adRequest(testOwner)
	req.PriceCents = 0
	id, err := usecase.NewCreateAdFunc(d)(ctx, req)
	require.NoError(t, err)
	_, err = usecase.NewCreateAdFunc(d)(ctx, adRequest(testOwner))
	require.NoError(t, err)
	ads, err := d.SearchAds(ctx, adsdao.SearchAdsRequest{MaxPriceSet: true, MaxPriceCents: 0})
	require.NoError(t, err)
	require.Len(t, ads, 1)
	require.Equal(t, id, ads[0].ID)
}

func TestIntegrationCreateIdempotency(t *testing.T) {
	d, dsn := integrationDAO(t)
	ctx := context.Background()
	d2, err := New(ctx, dsn)
	require.NoError(t, err)
	defer d2.Close()
	create := usecase.NewCreateAdFunc(d)
	// Leave only one slot: all concurrent retries must still succeed.
	for i := 0; i < 4; i++ {
		_, err = create(ctx, adRequest(testOwner))
		require.NoError(t, err)
	}
	req := adRequest(testOwner)
	req.IdempotencyKey = "same-key"
	funcs := []usecase.CreateAdFunc{create, usecase.NewCreateAdFunc(d2)}
	type result struct {
		id  domain.AdID
		err error
	}
	out := make(chan result, 20)
	start := make(chan struct{})
	for i := 0; i < 20; i++ {
		go func(i int) {
			<-start
			r := req
			if i%2 == 0 {
				r.OwnerID = domain.UserID(strings.ToUpper(string(r.OwnerID)))
				r.Title = "  bike  "
			}
			id, e := funcs[i%2](ctx, r)
			out <- result{id, e}
		}(i)
	}
	close(start)
	var first domain.AdID
	for i := 0; i < 20; i++ {
		r := <-out
		require.NoError(t, r.err)
		if i == 0 {
			first = r.id
		}
		require.Equal(t, first, r.id)
	}
	count, err := d.CountRecentPublications(ctx, testOwner, usecase.PublicationWindow)
	require.NoError(t, err)
	require.Equal(t, 5, count)
	var total int
	require.NoError(t, d.db.QueryRow("SELECT count(*) FROM ads WHERE user_id=$1", string(testOwner)).Scan(&total))
	require.Equal(t, 5, total)
	changed := req
	changed.PriceCents++
	_, err = create(ctx, changed)
	require.ErrorIs(t, err, usecase.ErrIdempotencyKeyReused)
	_, err = create(ctx, adRequest(testOwner))
	require.ErrorIs(t, err, usecase.ErrQuotaExceeded)
	// A quota rejection does not reserve the new key.
	denied := req
	denied.IdempotencyKey = "later"
	_, err = create(ctx, denied)
	require.ErrorIs(t, err, usecase.ErrQuotaExceeded)
	require.NoError(t, d.db.QueryRow("SELECT count(*) FROM ad_imports WHERE idempotency_key='later'").Scan(&total))
	require.Zero(t, total)
	// Operation and owner scopes remain independent.
	_, err = usecase.NewImportAdsFunc(d)(ctx, importRequest(testOwner, req.IdempotencyKey))
	require.NoError(t, err)
	other := req
	other.OwnerID = otherOwner
	otherID, err := create(ctx, other)
	require.NoError(t, err)
	require.NotEqual(t, first, otherID)
	require.NoError(t, d.DeleteAdByID(ctx, first, testOwner))
	replay, err := create(ctx, req)
	require.NoError(t, err)
	require.Equal(t, first, replay)
	_, err = d.GetAdByID(ctx, first)
	require.ErrorIs(t, err, adsdao.ErrNotFound)
}

func TestIntegrationCreateReceiptRollback(t *testing.T) {
	d, _ := integrationDAO(t)
	ctx := context.Background()
	req := adRequest(testOwner)
	req.IdempotencyKey = "retry-after-failure"
	// Fail after both ad and quota event have been inserted.
	_, err := d.db.Exec(`CREATE FUNCTION reject_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected receipt failure'; END $$;
 CREATE TRIGGER reject_receipt BEFORE UPDATE ON ad_imports FOR EACH ROW EXECUTE FUNCTION reject_receipt()`)
	require.NoError(t, err)
	create := usecase.NewCreateAdFunc(d)
	_, err = create(ctx, req)
	require.Error(t, err)
	for _, table := range []string{"ads", "ad_publications", "ad_imports"} {
		var n int
		require.NoError(t, d.db.QueryRow("SELECT count(*) FROM "+table).Scan(&n))
		require.Zero(t, n)
	}
	_, err = d.db.Exec("DROP TRIGGER reject_receipt ON ad_imports")
	require.NoError(t, err)
	id, err := create(ctx, req)
	require.NoError(t, err)
	again, err := create(ctx, req)
	require.NoError(t, err)
	require.Equal(t, id, again)
}

func TestIntegrationCreateKeyCancellation(t *testing.T) {
	d, _ := integrationDAO(t)
	ctx := context.Background()
	tx, err := d.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback()
	req := adRequest(testOwner)
	req.IdempotencyKey = "held"
	key := adsdao.RequestKey{OwnerID: testOwner, IdempotencyKey: req.IdempotencyKey, Operation: "create"}
	claimed, err := tx.ClaimRequest(ctx, key, "uncommitted")
	require.NoError(t, err)
	require.True(t, claimed)
	limited, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	_, err = usecase.NewCreateAdFunc(d)(limited, req)
	require.Error(t, err)
	require.ErrorIs(t, limited.Err(), context.DeadlineExceeded)
	require.NoError(t, tx.Rollback())
	id, err := usecase.NewCreateAdFunc(d)(ctx, req)
	require.NoError(t, err)
	require.Positive(t, int64(id))
}

func TestIntegrationReceiptMigrationPreservesImports(t *testing.T) {
	d, _ := integrationDAO(t)
	ctx := context.Background()
	req := importRequest(testOwner, "legacy-key")
	original, err := usecase.NewImportAdsFunc(d)(ctx, req)
	require.NoError(t, err)
	// Reconstruct the pre-006 receipt schema around a real committed import.
	_, err = d.db.Exec(`ALTER TABLE ad_imports DROP CONSTRAINT ad_imports_pkey;
 ALTER TABLE ad_imports DROP COLUMN operation;
 ALTER TABLE ad_imports ADD PRIMARY KEY (user_id,idempotency_key)`)
	require.NoError(t, err)
	migration, err := os.ReadFile("migrations/006_create_idempotency.sql")
	require.NoError(t, err)
	_, err = d.db.Exec(string(migration))
	require.NoError(t, err)
	replay, err := usecase.NewImportAdsFunc(d)(ctx, req)
	require.NoError(t, err)
	require.Equal(t, original, replay)
	single := adRequest(testOwner)
	single.IdempotencyKey = req.IdempotencyKey
	id, err := usecase.NewCreateAdFunc(d)(ctx, single)
	require.NoError(t, err)
	require.NotContains(t, original, id)
}
