package postgres

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	adsdao "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/dao"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
)

var adCols = []string{"id", "title", "price_cents", "photo_url", "user_id", "created_at"}

func newMockDAO(t *testing.T) (*DAO, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return &DAO{db: db}, mock
}

func TestSearchAds(t *testing.T) {
	ctx := context.Background()
	createdAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("returns matching ads", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		rows := sqlmock.NewRows(adCols).
			AddRow(int64(1), "bike", int64(5000), "https://example.com/bike.jpg", "00000000-0000-0000-0000-000000000001", createdAt).
			AddRow(int64(2), "scooter", int64(12000), "https://example.com/scooter.jpg", "00000000-0000-0000-0000-000000000002", createdAt)
		mock.ExpectQuery("SELECT").
			WithArgs(51).
			WillReturnRows(rows)

		got, err := dao.SearchAds(ctx, adsdao.SearchAdsRequest{})

		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, domain.AdID(1), got[0].ID)
		assert.Equal(t, "bike", got[0].Title)
		assert.Equal(t, int64(5000), got[0].Price)
		assert.Equal(t, domain.AdID(2), got[1].ID)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns nil when no ads match", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		mock.ExpectQuery("SELECT").
			WithArgs("%bike%", 51).
			WillReturnRows(sqlmock.NewRows(adCols))

		got, err := dao.SearchAds(ctx, adsdao.SearchAdsRequest{Title: "bike"})

		require.NoError(t, err)
		assert.Nil(t, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns error when query fails", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		boom := errors.New("db down")
		mock.ExpectQuery("SELECT").
			WithArgs(51).
			WillReturnError(boom)

		_, err := dao.SearchAds(ctx, adsdao.SearchAdsRequest{})

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGetAdByID(t *testing.T) {
	ctx := context.Background()
	createdAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("returns ad on success", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		rows := sqlmock.NewRows(adCols).
			AddRow(int64(42), "mountain bike", int64(15000), "https://example.com/bike.jpg", "00000000-0000-0000-0000-000000000001", createdAt)
		mock.ExpectQuery("SELECT").
			WithArgs(int64(42)).
			WillReturnRows(rows)

		got, err := dao.GetAdByID(ctx, 42)

		require.NoError(t, err)
		assert.Equal(t, domain.AdID(42), got.ID)
		assert.Equal(t, "mountain bike", got.Title)
		assert.Equal(t, int64(15000), got.Price)
		assert.Equal(t, domain.UserID("00000000-0000-0000-0000-000000000001"), got.UserID)
		assert.Equal(t, createdAt, got.CreatedAt)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns ErrNotFound when no row", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		mock.ExpectQuery("SELECT").
			WithArgs(int64(99)).
			WillReturnRows(sqlmock.NewRows(adCols))

		_, err := dao.GetAdByID(ctx, 99)

		require.Error(t, err)
		assert.ErrorIs(t, err, adsdao.ErrNotFound)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns error on query failure", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		boom := errors.New("connection reset")
		mock.ExpectQuery("SELECT").
			WithArgs(int64(1)).
			WillReturnError(boom)

		_, err := dao.GetAdByID(ctx, 1)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestCreateAd(t *testing.T) {
	ctx := context.Background()
	req := adsdao.CreateAdRequest{
		Title:      "mountain bike",
		PriceCents: 15000,
		PhotoURL:   "https://example.com/bike.jpg",
		OwnerID:    "00000000-0000-0000-0000-000000000001",
	}

	t.Run("returns generated ID on success", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		mock.ExpectQuery("INSERT").
			WithArgs(req.Title, req.PriceCents, req.PhotoURL, string(req.OwnerID)).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(7)))

		got, err := dao.CreateAd(ctx, req)

		require.NoError(t, err)
		assert.Equal(t, domain.AdID(7), got)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("preserves database constraint errors", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		pgErr := &pgconn.PgError{Code: "23503"}
		mock.ExpectQuery("INSERT").
			WithArgs(req.Title, req.PriceCents, req.PhotoURL, string(req.OwnerID)).
			WillReturnError(pgErr)

		_, err := dao.CreateAd(ctx, req)

		require.Error(t, err)
		assert.ErrorIs(t, err, pgErr)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns error on other failures", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		boom := errors.New("db down")
		mock.ExpectQuery("INSERT").
			WithArgs(req.Title, req.PriceCents, req.PhotoURL, string(req.OwnerID)).
			WillReturnError(boom)

		_, err := dao.CreateAd(ctx, req)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestDeleteAdByID(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes the ad", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		mock.ExpectExec("DELETE").
			WithArgs(int64(42), "00000000-0000-0000-0000-000000000001").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := dao.DeleteAdByID(ctx, 42, "00000000-0000-0000-0000-000000000001")

		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns ErrNotFound when no row is deleted", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		mock.ExpectExec("DELETE").
			WithArgs(int64(99), "00000000-0000-0000-0000-000000000001").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := dao.DeleteAdByID(ctx, 99, "00000000-0000-0000-0000-000000000001")

		require.Error(t, err)
		assert.ErrorIs(t, err, adsdao.ErrNotFound)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns error on query failure", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		boom := errors.New("connection reset")
		mock.ExpectExec("DELETE").
			WithArgs(int64(1), "00000000-0000-0000-0000-000000000001").
			WillReturnError(boom)

		err := dao.DeleteAdByID(ctx, 1, "00000000-0000-0000-0000-000000000001")

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns error when affected rows cannot be read", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		boom := errors.New("driver error")
		mock.ExpectExec("DELETE").
			WithArgs(int64(1), "00000000-0000-0000-0000-000000000001").
			WillReturnResult(sqlmock.NewErrorResult(boom))

		err := dao.DeleteAdByID(ctx, 1, "00000000-0000-0000-0000-000000000001")

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestTransaction(t *testing.T) {
	ctx := context.Background()
	req := adsdao.CreateAdRequest{
		Title:      "mountain bike",
		PriceCents: 15000,
		PhotoURL:   "https://example.com/bike.jpg",
		OwnerID:    "00000000-0000-0000-0000-000000000001",
	}

	t.Run("commits every call made on the transaction", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		mock.ExpectBegin()
		mock.ExpectQuery("INSERT").
			WithArgs(req.Title, req.PriceCents, req.PhotoURL, string(req.OwnerID)).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
		mock.ExpectQuery("INSERT").
			WithArgs(req.Title, req.PriceCents, req.PhotoURL, string(req.OwnerID)).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(2)))
		mock.ExpectCommit()

		tx, err := dao.Begin(ctx)
		require.NoError(t, err)

		first, err := tx.CreateAd(ctx, req)
		require.NoError(t, err)
		second, err := tx.CreateAd(ctx, req)
		require.NoError(t, err)

		require.NoError(t, tx.Commit())
		assert.Equal(t, domain.AdID(1), first)
		assert.Equal(t, domain.AdID(2), second)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("rolls back without committing", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		mock.ExpectBegin()
		mock.ExpectQuery("INSERT").
			WithArgs(req.Title, req.PriceCents, req.PhotoURL, string(req.OwnerID)).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
		mock.ExpectRollback()

		tx, err := dao.Begin(ctx)
		require.NoError(t, err)

		_, err = tx.CreateAd(ctx, req)
		require.NoError(t, err)

		require.NoError(t, tx.Rollback())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns error when the transaction cannot be opened", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		boom := errors.New("db down")
		mock.ExpectBegin().WillReturnError(boom)

		tx, err := dao.Begin(ctx)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
		assert.Nil(t, tx)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("rollback after commit does nothing", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		mock.ExpectBegin()
		mock.ExpectCommit()

		tx, err := dao.Begin(ctx)
		require.NoError(t, err)
		require.NoError(t, tx.Commit())

		require.NoError(t, tx.Rollback())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("rejects a nested transaction", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		mock.ExpectBegin()
		mock.ExpectRollback()

		tx, err := dao.Begin(ctx)
		require.NoError(t, err)

		_, nestedErr := tx.(*DAO).Begin(ctx)

		require.Error(t, nestedErr)
		require.NoError(t, tx.Rollback())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("refuses commit and rollback outside a transaction", func(t *testing.T) {
		dao, _ := newMockDAO(t)

		require.Error(t, dao.Commit())
		require.Error(t, dao.Rollback())
	})
}

// arrayConverter lets sqlmock accept the []int64 the DAO sends as a BIGINT[], which pgx encodes natively.
type arrayConverter struct{}

func (arrayConverter) ConvertValue(v any) (driver.Value, error) {
	if ids, ok := v.([]int64); ok {
		return ids, nil
	}

	return driver.DefaultParameterConverter.ConvertValue(v)
}

func newMockDAOWithArrays(t *testing.T) (*DAO, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.ValueConverterOption(arrayConverter{}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return &DAO{db: db}, mock
}

func TestClaimRequest(t *testing.T) {
	ctx := context.Background()
	key := adsdao.RequestKey{OwnerID: "00000000-0000-0000-0000-000000000001", IdempotencyKey: "key-1"}

	t.Run("returns true when the key is new", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		mock.ExpectExec("INSERT INTO ad_imports").
			WithArgs("00000000-0000-0000-0000-000000000001", "key-1", "hash-1", "import").
			WillReturnResult(sqlmock.NewResult(0, 1))

		reported, err := dao.ClaimRequest(ctx, key, "hash-1")

		require.NoError(t, err)
		assert.True(t, reported)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns false when the key is already recorded", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		mock.ExpectExec("INSERT INTO ad_imports").
			WithArgs("00000000-0000-0000-0000-000000000001", "key-1", "hash-1", "import").
			WillReturnResult(sqlmock.NewResult(0, 0))

		reported, err := dao.ClaimRequest(ctx, key, "hash-1")

		require.NoError(t, err)
		assert.False(t, reported)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns error on query failure", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		boom := errors.New("connection reset")
		mock.ExpectExec("INSERT INTO ad_imports").
			WithArgs("00000000-0000-0000-0000-000000000001", "key-1", "hash-1", "import").
			WillReturnError(boom)

		_, err := dao.ClaimRequest(ctx, key, "hash-1")

		assert.ErrorIs(t, err, boom)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns error when affected rows cannot be read", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		boom := errors.New("driver error")
		mock.ExpectExec("INSERT INTO ad_imports").
			WithArgs("00000000-0000-0000-0000-000000000001", "key-1", "hash-1", "import").
			WillReturnResult(sqlmock.NewErrorResult(boom))

		_, err := dao.ClaimRequest(ctx, key, "hash-1")

		assert.ErrorIs(t, err, boom)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestGetRequestReceipt(t *testing.T) {
	ctx := context.Background()
	key := adsdao.RequestKey{OwnerID: "00000000-0000-0000-0000-000000000001", IdempotencyKey: "key-1"}

	t.Run("returns the recorded import", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		mock.ExpectQuery("SELECT request_hash, ad_ids").
			WithArgs("00000000-0000-0000-0000-000000000001", "key-1", "import").
			WillReturnRows(sqlmock.NewRows([]string{"request_hash", "ad_ids"}).AddRow("hash-1", "{10,11}"))

		got, err := dao.GetRequestReceipt(ctx, key)

		require.NoError(t, err)
		assert.Equal(t, adsdao.RequestReceipt{RequestHash: "hash-1", AdIDs: []domain.AdID{10, 11}}, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns ErrNotFound when no row", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		mock.ExpectQuery("SELECT request_hash, ad_ids").
			WithArgs("00000000-0000-0000-0000-000000000001", "key-1", "import").
			WillReturnRows(sqlmock.NewRows([]string{"request_hash", "ad_ids"}))

		_, err := dao.GetRequestReceipt(ctx, key)

		assert.ErrorIs(t, err, adsdao.ErrNotFound)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns error on query failure", func(t *testing.T) {
		dao, mock := newMockDAO(t)
		boom := errors.New("connection reset")
		mock.ExpectQuery("SELECT request_hash, ad_ids").
			WithArgs("00000000-0000-0000-0000-000000000001", "key-1", "import").
			WillReturnError(boom)

		_, err := dao.GetRequestReceipt(ctx, key)

		assert.ErrorIs(t, err, boom)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestCompleteRequest(t *testing.T) {
	ctx := context.Background()
	key := adsdao.RequestKey{OwnerID: "00000000-0000-0000-0000-000000000001", IdempotencyKey: "key-1"}

	t.Run("stores the ad ids", func(t *testing.T) {
		dao, mock := newMockDAOWithArrays(t)
		mock.ExpectExec("UPDATE ad_imports").
			WithArgs("00000000-0000-0000-0000-000000000001", "key-1", []int64{10, 11}, "import").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := dao.CompleteRequest(ctx, key, []domain.AdID{10, 11})

		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns ErrNotFound when the import is not reported", func(t *testing.T) {
		dao, mock := newMockDAOWithArrays(t)
		mock.ExpectExec("UPDATE ad_imports").
			WithArgs("00000000-0000-0000-0000-000000000001", "key-1", []int64{10}, "import").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := dao.CompleteRequest(ctx, key, []domain.AdID{10})

		assert.ErrorIs(t, err, adsdao.ErrNotFound)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns error on query failure", func(t *testing.T) {
		dao, mock := newMockDAOWithArrays(t)
		boom := errors.New("connection reset")
		mock.ExpectExec("UPDATE ad_imports").
			WithArgs("00000000-0000-0000-0000-000000000001", "key-1", []int64{10}, "import").
			WillReturnError(boom)

		err := dao.CompleteRequest(ctx, key, []domain.AdID{10})

		assert.ErrorIs(t, err, boom)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns error when affected rows cannot be read", func(t *testing.T) {
		dao, mock := newMockDAOWithArrays(t)
		boom := errors.New("driver error")
		mock.ExpectExec("UPDATE ad_imports").
			WithArgs("00000000-0000-0000-0000-000000000001", "key-1", []int64{10}, "import").
			WillReturnResult(sqlmock.NewErrorResult(boom))

		err := dao.CompleteRequest(ctx, key, []domain.AdID{10})

		assert.ErrorIs(t, err, boom)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
