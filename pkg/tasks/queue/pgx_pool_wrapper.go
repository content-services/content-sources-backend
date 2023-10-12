package queue

import (
	"context"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

// PgxPoolWrapper wraps a pgx Pool in a generic interface to allow for alternative implementations, such as the FakePgxPoolWrapper
type PgxPoolWrapper struct {
	pool *pgxpool.Pool
}

func (p *PgxPoolWrapper) Begin(ctx context.Context) (pgx.Tx, error) {
	return p.pool.Begin(ctx)
}

func (p *PgxPoolWrapper) Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error) {
	return p.pool.Exec(ctx, sql, arguments...)
}

func (p *PgxPoolWrapper) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return p.pool.Query(ctx, sql, args...)
}

func (p *PgxPoolWrapper) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return p.pool.QueryRow(ctx, sql, args...)
}

func (p *PgxPoolWrapper) Acquire(ctx context.Context) (Connection, error) {
	conn, err := p.pool.Acquire(ctx)
	return &PgxConnWrapper{conn: conn}, err
}

func (p *PgxPoolWrapper) Stat() *pgxpool.Stat {
	// Not a real pool so ignore
	return p.pool.Stat()
}

// PgxConnWrapper wraps a pgxpool Conn in a generic interface to allow for alternative implementations, such as the FakePgxPoolWrapper
type PgxConnWrapper struct {
	conn *pgxpool.Conn
}

func (p *PgxConnWrapper) Begin(ctx context.Context) (pgx.Tx, error) {
	return p.conn.Begin(ctx)
}

func (p *PgxConnWrapper) Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error) {
	return p.conn.Exec(ctx, sql, arguments...)
}

func (p *PgxConnWrapper) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return p.conn.Query(ctx, sql, args...)
}

func (p *PgxConnWrapper) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return p.conn.QueryRow(ctx, sql, args...)
}

func (p *PgxConnWrapper) Release() {
	p.conn.Release()
}

func (p *PgxConnWrapper) Conn() *pgx.Conn {
	return p.conn.Conn()
}

// FakePgxPoolWrapper is used for testing, to provide a pool interface implementation that doesn't actually use the pool
// in order to wrap all work in a transaction.  This shouldn't be used in production, as it is based off a single
// transaction
type FakePgxPoolWrapper struct {
	tx   *pgx.Tx
	conn *pgxpool.Conn
}

func (p *FakePgxPoolWrapper) Release() {
	// This isn't a real pool so ignore
}

func (p *FakePgxPoolWrapper) Begin(ctx context.Context) (pgx.Tx, error) {
	return (*p.tx).Begin(ctx)
}

func (p *FakePgxPoolWrapper) Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error) {
	return (*p.tx).Exec(ctx, sql, arguments...)
}

func (p *FakePgxPoolWrapper) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return (*p.tx).Query(ctx, sql, args...)
}

func (p *FakePgxPoolWrapper) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return (*p.tx).QueryRow(ctx, sql, args...)
}

func (p *FakePgxPoolWrapper) Acquire(_ context.Context) (Connection, error) {
	// Not a real pool so ignore
	return p, nil
}

func (p *FakePgxPoolWrapper) Stat() *pgxpool.Stat {
	// Not a real pool so ignore
	return nil
}

func (p *FakePgxPoolWrapper) Conn() *pgx.Conn {
	return p.conn.Conn()
}
