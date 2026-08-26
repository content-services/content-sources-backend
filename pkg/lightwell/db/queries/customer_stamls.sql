-- name: CreateCustomerStaml :one
INSERT INTO lightwell_customer_stamls (customer_id, staml)
VALUES (sqlc.arg(customer_id), sqlc.arg(staml))
RETURNING customer_id, staml, created_at;

-- name: DeleteCustomerStaml :execrows
DELETE FROM lightwell_customer_stamls
WHERE customer_id = sqlc.arg(customer_id) AND staml = sqlc.arg(staml);
