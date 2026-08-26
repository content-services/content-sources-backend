package dao

import (
	"context"
	"errors"

	"github.com/content-services/content-sources-backend/pkg/api"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/lightwell/db/store"
	"github.com/jackc/pgx/v5/pgconn"
)

var _ LightwellCustomerStamlDao = lightwellCustomerStamlDaoImpl{}

type lightwellCustomerStamlDaoImpl struct {
	querier store.Querier
}

func newLightwellCustomerStamlDao(q store.Querier) LightwellCustomerStamlDao {
	return lightwellCustomerStamlDaoImpl{querier: q}
}

func (d lightwellCustomerStamlDaoImpl) Create(ctx context.Context, customerID, staml string) (api.LightwellCustomerStamlResponse, error) {
	row, err := d.querier.CreateCustomerStaml(ctx, store.CreateCustomerStamlParams{
		CustomerID: customerID,
		Staml:      staml,
	})
	if err != nil {
		return api.LightwellCustomerStamlResponse{}, customerStamlDBError(err)
	}
	return api.LightwellCustomerStamlResponse{
		CustomerID: row.CustomerID,
		Staml:      row.Staml,
		CreatedAt:  row.CreatedAt,
	}, nil
}

func (d lightwellCustomerStamlDaoImpl) Delete(ctx context.Context, customerID, staml string) error {
	n, err := d.querier.DeleteCustomerStaml(ctx, store.DeleteCustomerStamlParams{
		CustomerID: customerID,
		Staml:      staml,
	})
	if err != nil {
		return customerStamlDBError(err)
	}
	if n == 0 {
		return &ce.DaoError{NotFound: true, Message: "STAML to CID mapping not found"}
	}
	return nil
}

func customerStamlDBError(err error) *ce.DaoError {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "23505" {
		return &ce.DaoError{AlreadyExists: true, Message: "STAML to CID mapping already exists"}
	}
	return &ce.DaoError{Message: err.Error()}
}
