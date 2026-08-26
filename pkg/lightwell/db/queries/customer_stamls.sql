-- name: CreateCustomerStaml :one
INSERT INTO lightwell_customer_stamls (customer_id, staml)
VALUES (sqlc.arg(customer_id), sqlc.arg(staml))
RETURNING customer_id, staml, created_at;

-- name: DeleteCustomerStaml :execrows
DELETE FROM lightwell_customer_stamls
WHERE customer_id = sqlc.arg(customer_id) AND staml = sqlc.arg(staml);

-- name: ListCustomerIdsByStaml :many
SELECT DISTINCT customer_id
FROM lightwell_customer_stamls
WHERE staml = sqlc.arg(staml)
ORDER BY customer_id;

-- name: CustomerStamlExists :one
SELECT EXISTS(
    SELECT 1
    FROM lightwell_customer_stamls
    WHERE customer_id = sqlc.arg(customer_id) AND staml = sqlc.arg(staml)
);
