package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	_ "github.com/jackc/pgx/v5/stdlib"
	adsdao "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/dao"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
)

const adColumns = `id, title, price_cents, photo_url, user_id::text, created_at`

// DAO is a PostgreSQL-backed ad store.
type DAO struct {
	db *sql.DB
	// tx is set when the DAO is scoped to a transaction opened by Begin.
	tx *sql.Tx
}

// queryRunner is the subset of *sql.DB and *sql.Tx the queries need.
type queryRunner interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// New opens a connection pool to the given DSN and checks that the database is reachable. The caller is responsible for calling Close.
func New(ctx context.Context, dsn string) (*DAO, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("unable to open database: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("unable to reach database: %w", err)
	}

	return &DAO{db: db}, nil
}

// Close releases the connection pool.
func (d *DAO) Close() error {
	if err := d.db.Close(); err != nil {
		return fmt.Errorf("unable to close database: %w", err)
	}

	return nil
}

// runner returns the transaction the DAO is scoped to, or the connection pool.
func (d *DAO) runner() queryRunner {
	if d.tx != nil {
		return d.tx
	}

	return d.db
}

// Begin opens a transaction. Every call made on the returned Tx is part of it, and the caller must end it with Commit or Rollback.
func (d *DAO) Begin(ctx context.Context) (adsdao.Tx, error) {
	if d.tx != nil {
		return nil, errors.New("unable to open a nested transaction")
	}

	tx, err := d.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("unable to begin transaction: %w", err)
	}

	return &DAO{db: d.db, tx: tx}, nil
}

// Commit applies every change made through the transaction.
func (d *DAO) Commit() error {
	if d.tx == nil {
		return errors.New("unable to commit outside a transaction")
	}

	if err := d.tx.Commit(); err != nil {
		return fmt.Errorf("unable to commit transaction: %w", err)
	}

	return nil
}

// Rollback discards every change made through the transaction. Calling it on an already committed transaction does nothing, so `defer func() { _ = tx.Rollback() }()` is a safe guard.
func (d *DAO) Rollback() error {
	if d.tx == nil {
		return errors.New("unable to roll back outside a transaction")
	}

	if err := d.tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		return fmt.Errorf("unable to roll back transaction: %w", err)
	}

	return nil
}

// SearchAds returns all ads matching the filters.
func (d *DAO) SearchAds(ctx context.Context, req adsdao.SearchAdsRequest) ([]domain.Ad, error) {
	query := `SELECT ` + adColumns + ` FROM ads WHERE true`
	args := []any{}
	add := func(fragment string, value any) {
		args = append(args, value)
		query += fmt.Sprintf(fragment, len(args))
	}
	if req.OwnerID != "" {
		add(` AND user_id=$%d::uuid`, req.OwnerID)
	}
	if req.MinPriceCents != 0 {
		add(` AND price_cents >= $%d`, req.MinPriceCents)
	}
	if req.MaxPriceSet || req.MaxPriceCents != 0 {
		add(` AND price_cents <= $%d`, req.MaxPriceCents)
	}
	if req.Title != "" {
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(req.Title)
		add(` AND title ILIKE $%d`, "%"+escaped+"%")
	}
	if req.BeforeID > 0 {
		add(` AND id<$%d`, int64(req.BeforeID))
	}
	limit := req.Limit
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("%w: limit must be 1..100", domain.ErrInvalid)
	}
	query += ` ORDER BY id DESC`
	add(` LIMIT $%d`, limit+1)
	rows, err := d.runner().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("unable to search ads: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ads []domain.Ad
	for rows.Next() {
		var ad domain.Ad
		if err = rows.Scan(&ad.ID, &ad.Title, &ad.Price, &ad.PhotoURL, &ad.UserID, &ad.CreatedAt); err != nil {
			return nil, err
		}
		ads = append(ads, ad)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return ads, nil
}

// GetAdByID returns a single ad, or dao.ErrNotFound.
func (d *DAO) GetAdByID(ctx context.Context, id domain.AdID) (domain.Ad, error) {
	const query = `SELECT ` + adColumns + ` FROM ads WHERE id = $1`

	ad, err := scanAd(d.runner().QueryRowContext(ctx, query, int64(id)))
	if err != nil {
		return domain.Ad{}, fmt.Errorf("unable to get ad: %w", err)
	}

	return ad, nil
}

// CreateAd inserts an ad and returns its generated identifier.
func (d *DAO) CreateAd(ctx context.Context, req adsdao.CreateAdRequest) (domain.AdID, error) {
	const query = `
		INSERT INTO ads (title, price_cents, photo_url, user_id)
		VALUES ($1, $2, $3, $4::uuid)
		RETURNING id`

	var id domain.AdID
	err := d.runner().QueryRowContext(ctx, query, req.Title, req.PriceCents, req.PhotoURL, string(req.OwnerID)).Scan(&id)
	if err != nil {

		return 0, fmt.Errorf("unable to create ad: %w", err)
	}

	return id, nil
}

// DeleteAd removes a single ad.
func (d *DAO) DeleteAdByID(ctx context.Context, id domain.AdID, ownerID domain.UserID) error {
	const query = `DELETE FROM ads WHERE id = $1 AND user_id = $2::uuid`

	res, err := d.runner().ExecContext(ctx, query, int64(id), string(ownerID))
	if err != nil {
		return fmt.Errorf("unable to delete ad: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("unable to delete ad: %w", err)
	}

	if affected == 0 {
		return fmt.Errorf("unable to delete ad: %w", adsdao.ErrNotFound)
	}

	return nil
}

// ClaimRequest claims a request with its operation-scoped idempotency key. It returns false when the key is already recorded.
// Used on first request to claim the request, and on retries to check that the same content is being retried.
func (d *DAO) ClaimRequest(ctx context.Context, key adsdao.RequestKey, requestHash string) (bool, error) {
	const query = `
		INSERT INTO ad_imports (user_id, idempotency_key, request_hash, operation)
		VALUES ($1::uuid, $2, $3, $4)
		ON CONFLICT (user_id, operation, idempotency_key) DO NOTHING`

	res, err := d.runner().ExecContext(ctx, query, string(key.OwnerID), key.IdempotencyKey, requestHash, receiptOperation(key))
	if err != nil {
		return false, fmt.Errorf("unable to claim request: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("unable to claim request: %w", err)
	}

	return affected == 1, nil
}

// GetRequestReceipt returns a recorded request receipt, or dao.ErrNotFound.
func (d *DAO) GetRequestReceipt(ctx context.Context, key adsdao.RequestKey) (adsdao.RequestReceipt, error) {
	const query = `
		SELECT request_hash, ad_ids
		FROM ad_imports
		WHERE user_id = $1::uuid AND idempotency_key = $2 AND operation = $3`

	var (
		receipt adsdao.RequestReceipt
		rawIDs  []int64
	)

	err := d.runner().QueryRowContext(ctx, query, string(key.OwnerID), key.IdempotencyKey, receiptOperation(key)).
		Scan(&receipt.RequestHash, pgtype.NewMap().SQLScanner(&rawIDs))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return adsdao.RequestReceipt{}, fmt.Errorf("unable to get request receipt: %w", adsdao.ErrNotFound)
		}

		return adsdao.RequestReceipt{}, fmt.Errorf("unable to get request receipt: %w", err)
	}

	receipt.AdIDs = make([]domain.AdID, 0, len(rawIDs))
	for _, id := range rawIDs {
		receipt.AdIDs = append(receipt.AdIDs, domain.AdID(id))
	}

	return receipt, nil
}

// CompleteRequest stores the ids of the ads created by a claimed request, or returns dao.ErrNotFound.
// Called in the same transaction as the created ads, before commit.
func (d *DAO) CompleteRequest(ctx context.Context, key adsdao.RequestKey, adIDs []domain.AdID) error {
	const query = `
		UPDATE ad_imports
		SET ad_ids = $3
		WHERE user_id = $1::uuid AND idempotency_key = $2 AND operation = $4`

	rawIDs := make([]int64, 0, len(adIDs))
	for _, id := range adIDs {
		rawIDs = append(rawIDs, int64(id))
	}

	res, err := d.runner().ExecContext(ctx, query, string(key.OwnerID), key.IdempotencyKey, rawIDs, receiptOperation(key))
	if err != nil {
		return fmt.Errorf("unable to complete request: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("unable to complete request: %w", err)
	}

	if affected == 0 {
		return fmt.Errorf("unable to complete request: %w", adsdao.ErrNotFound)
	}

	return nil
}

func scanAd(row *sql.Row) (domain.Ad, error) {
	var ad domain.Ad
	if err := row.Scan(&ad.ID, &ad.Title, &ad.Price, &ad.PhotoURL, &ad.UserID, &ad.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Ad{}, adsdao.ErrNotFound
		}

		return domain.Ad{}, err
	}

	return ad, nil
}

func (d *DAO) LockOwnerPublications(ctx context.Context, ownerID domain.UserID) error {
	if d.tx == nil {
		return errors.New("unable to lock publications outside a transaction")
	}

	// The lock is released automatically when the transaction commits or rolls back. The prefix keeps it apart from any other advisory lock.
	const query = `SELECT pg_advisory_xact_lock(hashtextextended('ad_publications:' || $1::uuid::text, 713))`

	if _, err := d.tx.ExecContext(ctx, query, string(ownerID)); err != nil {
		return fmt.Errorf("unable to lock publications: %w", err)
	}

	return nil
}

func (d *DAO) CountRecentPublications(ctx context.Context, ownerID domain.UserID, window time.Duration) (int, error) {
	const query = `
		SELECT count(*)
		FROM ad_publications
		WHERE owner_id = $1::uuid
		  AND published_at > statement_timestamp() - ($2::float8 * interval '1 second')`

	var count int
	if err := d.runner().QueryRowContext(ctx, query, string(ownerID), window.Seconds()).Scan(&count); err != nil {
		return 0, fmt.Errorf("unable to count publications: %w", err)
	}

	return count, nil
}

func (d *DAO) RecordPublication(ctx context.Context, ownerID domain.UserID, adID domain.AdID) error {
	const query = `INSERT INTO ad_publications (owner_id, ad_id) VALUES ($1::uuid, $2)`

	if _, err := d.runner().ExecContext(ctx, query, string(ownerID), int64(adID)); err != nil {
		return fmt.Errorf("unable to record publication: %w", err)
	}

	return nil
}

func receiptOperation(key adsdao.RequestKey) string {
	if key.Operation == "" {
		return "import"
	}
	return key.Operation
}
