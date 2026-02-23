package store

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"

	"github.com/A-pen-app/feed-sdk/model"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
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

const createColdstartTableSQL = `
CREATE TABLE IF NOT EXISTS feed_coldstart (
	feed_id uuid NOT NULL,
	position integer NOT NULL DEFAULT 0,
	feed_type character varying(20) NOT NULL DEFAULT 'banners'::character varying,
	CONSTRAINT feed_coldstart_pkey PRIMARY KEY (feed_id),
	CONSTRAINT feed_coldstart_position_key UNIQUE (position) INCLUDE (position)
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
				IF p !~ '^(exposure|inexpose|unexpose|istarget|istheone):[a-z0-9:_-]+$' THEN
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

	if _, err := db.Exec(createColdstartTableSQL); err != nil {
		panic("failed to create feed_coldstart table: " + err.Error())
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

func (f *store) GetColdstart(ctx context.Context) ([]model.Policy, error) {
	orders := []model.Policy{}

	if err := f.db.Select(
		&orders,
		`
		SELECT
			feed_coldstart.feed_id,
			feed_coldstart.feed_type,
			feed_coldstart.position
		FROM
			feed_coldstart
		ORDER BY
			feed_coldstart.position ASC
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
	tx, err := f.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get the feed to be deleted
	var deletedFeed struct {
		FeedType string `db:"feed_type"`
		Position int    `db:"position"`
	}
	err = tx.GetContext(ctx, &deletedFeed,
		`SELECT feed_type, position FROM feed WHERE feed_id = $1 FOR UPDATE`, id)
	if err != nil {
		// Feed not found or error — attempt simple delete
		if _, err := tx.ExecContext(ctx, `DELETE FROM feed WHERE feed_id = $1`, id); err != nil {
			return err
		}
		return tx.Commit()
	}

	// Only promote a replacement for 'posts' type
	if model.FeedType(deletedFeed.FeedType) != model.TypePosts {
		if _, err := tx.ExecContext(ctx, `DELETE FROM feed WHERE feed_id = $1`, id); err != nil {
			return err
		}
		return tx.Commit()
	}

	// Look for a replacement candidate in feed_relation where related_feed_id = source_id
	var replacement struct {
		FeedID   string         `db:"feed_id"`
		Policies pq.StringArray `db:"policies"`
	}
	err = tx.GetContext(ctx, &replacement,
		`SELECT feed_id, policies FROM feed_relation WHERE related_feed_id = $1 LIMIT 1`, id)
	if err != nil {
		// No replacement available, simple delete
		if _, err := tx.ExecContext(ctx, `DELETE FROM feed WHERE feed_id = $1`, id); err != nil {
			return err
		}
		return tx.Commit()
	}

	// 1. Delete the selected relation row
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM feed_relation WHERE feed_id = $1 AND related_feed_id = $2`,
		replacement.FeedID, id); err != nil {
		return err
	}

	// 2. Update remaining relations: point related_feed_id from source_id to replacement
	if _, err := tx.ExecContext(ctx,
		`UPDATE feed_relation SET related_feed_id = $1 WHERE related_feed_id = $2`,
		replacement.FeedID, id); err != nil {
		return err
	}

	// 3. Delete the original feed
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM feed WHERE feed_id = $1`, id); err != nil {
		return err
	}

	// 4. Insert the replacement feed at the same position
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO feed (feed_id, feed_type, position, policies)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (feed_id) DO UPDATE SET
			 feed_type = EXCLUDED.feed_type,
			 position = EXCLUDED.position,
			 policies = EXCLUDED.policies`,
		replacement.FeedID, model.TypePosts, deletedFeed.Position, replacement.Policies); err != nil {
		return err
	}

	return tx.Commit()
}

func (f *store) CreateFeedPosition(ctx context.Context, feedID string, feedType model.FeedType, position int, policies pq.StringArray) error {
	if feedType == model.TypeBanners {
		_, err := f.db.NamedExecContext(ctx,
			`INSERT INTO feed (feed_id, feed_type, position, policies)
			 VALUES (:feed_id, :feed_type, :position, :policies)`,
			map[string]interface{}{
				"feed_id":   feedID,
				"feed_type": feedType,
				"position":  position,
				"policies":  policies,
			})
		return err
	}

	// feed_type == "post"
	tx, err := f.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var existing struct {
		FeedID   string `db:"feed_id"`
		FeedType string `db:"feed_type"`
	}
	err = tx.GetContext(ctx, &existing,
		`SELECT feed_id, feed_type FROM feed WHERE position = $1 FOR UPDATE`, position)
	if err != nil {
		// Position empty — insert directly
		_, err = tx.NamedExecContext(ctx,
			`INSERT INTO feed (feed_id, feed_type, position, policies)
			 VALUES (:feed_id, :feed_type, :position, :policies)`,
			map[string]interface{}{
				"feed_id":   feedID,
				"feed_type": feedType,
				"position":  position,
				"policies":  policies,
			})
		if err != nil {
			return err
		}
		return tx.Commit()
	}

	existingType := model.FeedType(existing.FeedType)

	if existingType == model.TypeBanners {
		return fmt.Errorf("position %d is occupied by banners", position)
	}

	// Existing type is "post" or "posts" — add relation
	if err := f.AddRelationWithPolicies(ctx, tx, feedID, existing.FeedID, policies); err != nil {
		return err
	}

	// Upgrade to "posts" if the existing entry is still "post"
	if existingType == model.TypePost {
		if _, err := tx.ExecContext(ctx,
			`UPDATE feed SET feed_type = $1 WHERE feed_id = $2`,
			model.TypePosts, existing.FeedID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (f *store) DeleteFeedPosition(ctx context.Context, feedID string, position int) error {
	tx, err := f.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check if there is a row matching feed_id AND position
	var existing struct {
		FeedID   string `db:"feed_id"`
		FeedType string `db:"feed_type"`
	}
	err = tx.GetContext(ctx, &existing,
		`SELECT feed_id, feed_type FROM feed WHERE feed_id = $1 AND position = $2 FOR UPDATE`,
		feedID, position)
	if err != nil {
		// No row matching both feed_id and position — look in feed_relation instead
		var positionHolder struct {
			FeedID string `db:"feed_id"`
		}
		if err := tx.GetContext(ctx, &positionHolder,
			`SELECT feed_id FROM feed WHERE position = $1 FOR UPDATE`, position); err != nil {
			return fmt.Errorf("no feed found at position %d: %w", position, err)
		}

		if _, err := tx.ExecContext(ctx,
			`DELETE FROM feed_relation WHERE feed_id = $1 AND related_feed_id = $2`,
			feedID, positionHolder.FeedID); err != nil {
			return err
		}

		return tx.Commit()
	}

	// Row found — check feed_type
	if model.FeedType(existing.FeedType) != model.TypePosts {
		// Simple delete
		if _, err := tx.ExecContext(ctx, `DELETE FROM feed WHERE feed_id = $1`, feedID); err != nil {
			return err
		}
		return tx.Commit()
	}

	// feed_type is "posts" — pick a random replacement from feed_relation
	var replacement struct {
		FeedID   string         `db:"feed_id"`
		Policies pq.StringArray `db:"policies"`
	}
	err = tx.GetContext(ctx, &replacement,
		`SELECT feed_id, policies FROM feed_relation WHERE related_feed_id = $1 ORDER BY RANDOM() LIMIT 1`,
		feedID)
	if err != nil {
		// No replacement available, simple delete
		if _, err := tx.ExecContext(ctx, `DELETE FROM feed WHERE feed_id = $1`, feedID); err != nil {
			return err
		}
		return tx.Commit()
	}

	// 1. Delete the selected relation row
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM feed_relation WHERE feed_id = $1 AND related_feed_id = $2`,
		replacement.FeedID, feedID); err != nil {
		return err
	}

	// 2. Update remaining relations: point related_feed_id from old to replacement
	if _, err := tx.ExecContext(ctx,
		`UPDATE feed_relation SET related_feed_id = $1 WHERE related_feed_id = $2`,
		replacement.FeedID, feedID); err != nil {
		return err
	}

	// 3. Delete the original feed
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM feed WHERE feed_id = $1`, feedID); err != nil {
		return err
	}

	// 4. Insert the replacement feed at the same position
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO feed (feed_id, feed_type, position, policies)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (feed_id) DO UPDATE SET
			 feed_type = EXCLUDED.feed_type,
			 position = EXCLUDED.position,
			 policies = EXCLUDED.policies`,
		replacement.FeedID, model.TypePosts, position, replacement.Policies); err != nil {
		return err
	}

	return tx.Commit()
}

// LoadColdstartFromCSV reads a CSV file and loads feed IDs into the feed_coldstart table.
// The CSV is expected to have a header row, with the first column containing UUIDs.
// Each row is inserted with feed_type='post' and an incremental position starting from 0.
func (f *store) LoadColdstartFromCSV(ctx context.Context, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)

	// Skip header row
	if _, err := reader.Read(); err != nil {
		return fmt.Errorf("failed to read CSV header: %w", err)
	}

	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read CSV records: %w", err)
	}

	// Use a transaction for atomicity
	tx, err := f.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Clear existing coldstart data
	if _, err := tx.ExecContext(ctx, "DELETE FROM feed_coldstart"); err != nil {
		return fmt.Errorf("failed to clear existing coldstart data: %w", err)
	}

	// Insert each record with incremental position
	for position, record := range records {
		if len(record) == 0 {
			continue
		}

		feedID := record[0]
		if feedID == "" {
			continue
		}

		_, err := tx.NamedExecContext(ctx,
			`INSERT INTO feed_coldstart (feed_id, feed_type, position)
			 VALUES (:feed_id, :feed_type, :position)`,
			map[string]interface{}{
				"feed_id":   feedID,
				"feed_type": model.TypePost,
				"position":  position,
			})
		if err != nil {
			return fmt.Errorf("failed to insert feed_id %s at position %d: %w", feedID, position, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
