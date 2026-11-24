-- name: UpsertItemPrice :exec
-- Keeps the newer trade of each side: a side only moves forward in time, and
-- a side the batch lacks keeps what is stored. The limit and members stay
-- when the batch does not know them.
INSERT INTO item_prices (item_id, high, high_at, low, low_at, buy_limit, members, source, fetched_at)
VALUES (
  sqlc.arg(item_id), sqlc.narg(high), sqlc.narg(high_at), sqlc.narg(low), sqlc.narg(low_at),
  sqlc.narg(buy_limit), sqlc.narg(members), sqlc.arg(source), sqlc.arg(fetched_at)
)
ON CONFLICT (item_id) DO UPDATE SET
  high = CASE WHEN EXCLUDED.high_at IS NOT NULL AND (item_prices.high_at IS NULL OR EXCLUDED.high_at >= item_prices.high_at)
              THEN EXCLUDED.high ELSE item_prices.high END,
  high_at = CASE WHEN EXCLUDED.high_at IS NOT NULL AND (item_prices.high_at IS NULL OR EXCLUDED.high_at >= item_prices.high_at)
                 THEN EXCLUDED.high_at ELSE item_prices.high_at END,
  low = CASE WHEN EXCLUDED.low_at IS NOT NULL AND (item_prices.low_at IS NULL OR EXCLUDED.low_at >= item_prices.low_at)
             THEN EXCLUDED.low ELSE item_prices.low END,
  low_at = CASE WHEN EXCLUDED.low_at IS NOT NULL AND (item_prices.low_at IS NULL OR EXCLUDED.low_at >= item_prices.low_at)
                THEN EXCLUDED.low_at ELSE item_prices.low_at END,
  buy_limit = COALESCE(EXCLUDED.buy_limit, item_prices.buy_limit),
  members = COALESCE(EXCLUDED.members, item_prices.members),
  source = EXCLUDED.source,
  fetched_at = GREATEST(EXCLUDED.fetched_at, item_prices.fetched_at);

-- name: GetItemPrices :many
SELECT item_id, high, high_at, low, low_at, buy_limit, members, source, fetched_at
FROM item_prices
WHERE item_id = ANY(sqlc.arg(item_ids)::int[])
ORDER BY item_id;
