package store

import (
	"context"

	"github.com/A-pen-app/feed-sdk/model"
	"github.com/jmoiron/sqlx"
)

const createTableSQL = `
CREATE TABLE IF NOT EXISTS feed (
	feed_id uuid NOT NULL,
	position integer NOT NULL DEFAULT 0,
	feed_type character varying(20) NOT NULL DEFAULT 'banners'::character varying,
	policies character varying(50)[] NOT NULL DEFAULT ARRAY[]::character varying[],
	CONSTRAINT feed_pkey PRIMARY KEY (feed_id),
	CONSTRAINT feed_position_position1_key UNIQUE (position) INCLUDE (position)
)`

// addPolicyFormatConstraintSQL adds a CHECK constraint to validate policy format.
// Policies must be hyphen-separated with a valid policy type prefix.
// To update this constraint when adding new policy types:
//  1. Add the new policy type to the regex pattern
//  2. Run the migration (it will drop and recreate the constraint)
const addPolicyFormatConstraintSQL = `
DO $$
BEGIN
	-- Drop existing constraint if it exists (allows updating policy types)
	ALTER TABLE feed DROP CONSTRAINT IF EXISTS policies_format_check;

	-- Add constraint that validates each policy in the array
	-- Format: {policy_type}-{params} where policy_type is one of the known types
	ALTER TABLE feed ADD CONSTRAINT policies_format_check CHECK (
		policies = ARRAY[]::character varying[]
		OR NOT EXISTS (
			SELECT 1 FROM unnest(policies) AS p
			WHERE p !~ '^(exposure|inexpose|unexpose|istarget)-[a-z0-9-]+$'
		)
	);
END $$;
`

func NewFeed(db *sqlx.DB) *store {
	if db == nil {
		panic("database connection cannot be nil")
	}

	if _, err := db.Exec(createTableSQL); err != nil {
		panic("failed to create feed table: " + err.Error())
	}

	if _, err := db.Exec(addPolicyFormatConstraintSQL); err != nil {
		panic("failed to add policy format constraint: " + err.Error())
	}

	return &store{
		db: db,
	}
}

type store struct {
	db *sqlx.DB
}

func (f *store) GetPolicies(ctx context.Context) ([]model.Policy, error) {
	orders := []model.Policy{}

	if err := f.db.Select(
		&orders,
		`
		SELECT
			feed.feed_id,
			feed.feed_type,
			feed.position,
			feed.policies
		FROM
			feed
		ORDER BY
			feed.position ASC
		`,
	); err != nil {
		return nil, err
	}

	return orders, nil
}

func (f *store) PatchFeed(ctx context.Context, id string, feed_type model.FeedType, position int) error {
	_, err := f.db.NamedExec(
		`
		INSERT INTO 
			feed 
			(
				feed_id, 
				feed_type,
				position
			)
		VALUES 
			(
				:feed_id,
				:feed_type,
				:position
			)
		ON CONFLICT
			(feed_id) 
		DO UPDATE SET 
			feed_type = :feed_type,
			position = :position
		`,
		map[string]interface{}{
			"feed_id":   id,
			"feed_type": feed_type,
			"position":  position,
		})
	return err
}

func (f *store) DeleteFeed(ctx context.Context, id string) error {
	_, err := f.db.NamedExec(
		`
		DELETE FROM
			feed 
		WHERE 
			feed_id=:feed_id
		`,
		map[string]interface{}{
			"feed_id": id,
		})
	return err
}
