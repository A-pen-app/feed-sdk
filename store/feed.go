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
	policies character varying(200)[] NOT NULL DEFAULT ARRAY[]::character varying[],
	CONSTRAINT feed_pkey PRIMARY KEY (feed_id),
	CONSTRAINT feed_position_position1_key UNIQUE (position) INCLUDE (position)
)`

const createFeedChangelogTableSQL = `
CREATE TABLE IF NOT EXISTS feed_changelog (
	id SERIAL PRIMARY KEY,
	feed_id uuid NOT NULL,
	change_type character varying(20) NOT NULL,
	old_feed_type character varying(20),
	new_feed_type character varying(20),
	old_position integer,
	new_position integer,
	old_policies character varying(200)[],
	new_policies character varying(200)[],
	changed_at timestamp with time zone NOT NULL DEFAULT NOW()
)`

const createFeedChangelogTriggerSQL = `
DO $$
BEGIN
	-- Create or replace the changelog trigger function
	CREATE OR REPLACE FUNCTION log_feed_changes()
	RETURNS TRIGGER AS $func$
	DECLARE
		change_type_val TEXT;
	BEGIN
		IF TG_OP = 'INSERT' THEN
			INSERT INTO feed_changelog (feed_id, change_type, new_feed_type, new_position, new_policies)
			VALUES (NEW.feed_id, 'INSERT', NEW.feed_type, NEW.position, NEW.policies);
			RETURN NEW;
		ELSIF TG_OP = 'DELETE' THEN
			INSERT INTO feed_changelog (feed_id, change_type, old_feed_type, old_position, old_policies)
			VALUES (OLD.feed_id, 'DELETE', OLD.feed_type, OLD.position, OLD.policies);
			RETURN OLD;
		ELSIF TG_OP = 'UPDATE' THEN
			-- Only log if something actually changed
			IF OLD.feed_type IS DISTINCT FROM NEW.feed_type OR
			   OLD.position IS DISTINCT FROM NEW.position OR
			   OLD.policies IS DISTINCT FROM NEW.policies
			THEN
				-- Determine change type, prioritizing policy changes
				IF OLD.policies IS DISTINCT FROM NEW.policies THEN
					-- Use cardinality for cleaner array size comparison (returns 0 for empty arrays)
					IF cardinality(NEW.policies) > cardinality(OLD.policies) THEN
						change_type_val := 'POLICY_ADD';
					ELSIF cardinality(NEW.policies) < cardinality(OLD.policies) THEN
						change_type_val := 'POLICY_DELETE';
					ELSE
						change_type_val := 'POLICY_MODIFY';
					END IF;
				ELSE
					change_type_val := 'UPDATE';
				END IF;

				INSERT INTO feed_changelog (feed_id, change_type, old_feed_type, new_feed_type, old_position, new_position, old_policies, new_policies)
				VALUES (NEW.feed_id, change_type_val, OLD.feed_type, NEW.feed_type, OLD.position, NEW.position, OLD.policies, NEW.policies);
			END IF;
			RETURN NEW;
		END IF;
		RETURN NULL;
	END;
	$func$ LANGUAGE plpgsql;

	-- Drop existing trigger if it exists
	DROP TRIGGER IF EXISTS feed_changelog_trigger ON feed;

	-- Create the trigger
	CREATE TRIGGER feed_changelog_trigger
		AFTER INSERT OR UPDATE OR DELETE ON feed
		FOR EACH ROW
		EXECUTE FUNCTION log_feed_changes();
END $$;
`

// addPolicyFormatConstraintSQL creates a trigger function and trigger to validate policy format.
// Policies must be colon-separated with a valid policy type prefix.
// To update this constraint when adding new policy types:
//  1. Add the new policy type to the regex pattern in the function
//  2. Run the migration (it will replace the function)
const addPolicyFormatConstraintSQL = `
DO $$
BEGIN
	-- Create or replace the validation function
	CREATE OR REPLACE FUNCTION validate_policies_format()
	RETURNS TRIGGER AS $func$
	DECLARE
		p TEXT;
	BEGIN
		IF NEW.policies IS NOT NULL AND array_length(NEW.policies, 1) > 0 THEN
			FOREACH p IN ARRAY NEW.policies LOOP
				IF p !~ '^(exposure|inexpose|unexpose|istarget|istheone):[a-z0-9:-]+$' THEN
					RAISE EXCEPTION 'Invalid policy format: %. Must match pattern {policy_type}:{params}', p;
				END IF;
			END LOOP;
		END IF;
		RETURN NEW;
	END;
	$func$ LANGUAGE plpgsql;

	-- Drop existing trigger if it exists
	DROP TRIGGER IF EXISTS policies_format_trigger ON feed;

	-- Create the trigger
	CREATE TRIGGER policies_format_trigger
		BEFORE INSERT OR UPDATE ON feed
		FOR EACH ROW
		EXECUTE FUNCTION validate_policies_format();
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

	if _, err := db.Exec(createFeedRelationTableSQL); err != nil {
		panic("failed to create feed_relation table: " + err.Error())
	}

	if _, err := db.Exec(createFeedChangelogTableSQL); err != nil {
		panic("failed to create feed_changelog table: " + err.Error())
	}

	if _, err := db.Exec(createFeedChangelogTriggerSQL); err != nil {
		panic("failed to create feed_changelog trigger: " + err.Error())
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
