CREATE TABLE IF NOT EXISTS bids (
  id BIGSERIAL PRIMARY KEY,
  auction_id BIGINT NOT NULL,
  bidder_id BIGINT NOT NULL,
  amount NUMERIC(12,2) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_bids_auction_id ON bids(auction_id);
