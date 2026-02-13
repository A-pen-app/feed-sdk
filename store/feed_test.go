package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/A-pen-app/feed-sdk/model"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

func newMockStore(t *testing.T) (*store, sqlmock.Sqlmock, func()) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create mock db: %v", err)
	}

	mock.ExpectExec("CREATE TABLE IF NOT EXISTS feed").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS feed_coldstart").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DO \\$\\$").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS feed_relation").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS feed_changelog").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DO \\$\\$").WillReturnResult(sqlmock.NewResult(0, 0))

	sqlxDB := sqlx.NewDb(db, "postgres")
	s := NewFeed(sqlxDB)

	return s, mock, func() { db.Close() }
}

func TestNewFeed(t *testing.T) {
	t.Run("panics with nil database", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic with nil database, but did not panic")
			}
		}()
		NewFeed(nil)
	})

	t.Run("creates store with valid database", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock db: %v", err)
		}
		defer db.Close()

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS feed").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS feed_coldstart").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DO \\$\\$").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS feed_relation").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS feed_changelog").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DO \\$\\$").WillReturnResult(sqlmock.NewResult(0, 0))

		sqlxDB := sqlx.NewDb(db, "postgres")
		store := NewFeed(sqlxDB)

		if store == nil {
			t.Fatal("expected store to be created, got nil")
		}
		if store.db == nil {
			t.Error("expected store.db to be set, got nil")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("panics on migration error", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic on migration error, but did not panic")
			}
		}()

		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock db: %v", err)
		}
		defer db.Close()

		mock.ExpectExec("CREATE TABLE IF NOT EXISTS feed").WillReturnError(sqlmock.ErrCancelled)

		sqlxDB := sqlx.NewDb(db, "postgres")
		NewFeed(sqlxDB)
	})
}

func TestGetPolicies(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		mockRows       *sqlmock.Rows
		mockError      error
		expectedCount  int
		expectedError  bool
		validateResult func(t *testing.T, policies []model.Policy)
	}{
		{
			name: "successful query with multiple policies",
			mockRows: sqlmock.NewRows([]string{"feed_id", "feed_type", "position", "policies"}).
				AddRow("feed1", "post", 0, pq.StringArray{"exposure:1000"}).
				AddRow("feed2", "banners", 1, pq.StringArray{"inexpose:1234567890", "unexpose:9876543210"}).
				AddRow("feed3", "chat", 2, pq.StringArray{"exposure:500"}),
			expectedCount: 3,
			validateResult: func(t *testing.T, policies []model.Policy) {
				if policies[0].FeedId != "feed1" {
					t.Errorf("expected first feed ID 'feed1', got '%s'", policies[0].FeedId)
				}
				if policies[0].FeedType != model.TypePost {
					t.Errorf("expected first feed type 'post', got '%s'", policies[0].FeedType)
				}
				if policies[0].Position != 0 {
					t.Errorf("expected first position 0, got %d", policies[0].Position)
				}
				if len(policies[0].Policies) != 1 {
					t.Errorf("expected 1 policy for feed1, got %d", len(policies[0].Policies))
				}

				if policies[1].FeedId != "feed2" {
					t.Errorf("expected second feed ID 'feed2', got '%s'", policies[1].FeedId)
				}
				if len(policies[1].Policies) != 2 {
					t.Errorf("expected 2 policies for feed2, got %d", len(policies[1].Policies))
				}
			},
		},
		{
			name:          "empty result set",
			mockRows:      sqlmock.NewRows([]string{"feed_id", "feed_type", "position", "policies"}),
			expectedCount: 0,
		},
		{
			name:          "database error",
			mockError:     sqlmock.ErrCancelled,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, mock, cleanup := newMockStore(t)
			defer cleanup()

			// Set up expectations
			if tt.mockError != nil {
				mock.ExpectQuery("SELECT").WillReturnError(tt.mockError)
			} else {
				mock.ExpectQuery("SELECT").WillReturnRows(tt.mockRows)
			}

			policies, err := store.GetPolicies(ctx)

			if tt.expectedError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(policies) != tt.expectedCount {
				t.Fatalf("expected %d policies, got %d", tt.expectedCount, len(policies))
			}

			if tt.validateResult != nil {
				tt.validateResult(t, policies)
			}

			// Verify all expectations were met
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestPatchFeed(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		feedID        string
		feedType      model.FeedType
		position      int
		mockError     error
		expectedError bool
	}{
		{
			name:     "successful insert",
			feedID:   "feed123",
			feedType: model.TypePost,
			position: 5,
		},
		{
			name:     "successful update on conflict",
			feedID:   "feed456",
			feedType: model.TypeBanners,
			position: 10,
		},
		{
			name:          "database error",
			feedID:        "feed789",
			feedType:      model.TypeChat,
			position:      15,
			mockError:     sqlmock.ErrCancelled,
			expectedError: true,
		},
		{
			name:     "position zero",
			feedID:   "feed000",
			feedType: model.TypePost,
			position: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, mock, cleanup := newMockStore(t)
			defer cleanup()

			// Set up expectations
			// Note: ON CONFLICT DO UPDATE uses named parameters which get converted to positional.
			// The parameters appear twice: once for INSERT, once for UPDATE clause.
			if tt.mockError != nil {
				mock.ExpectExec("INSERT INTO feed").WillReturnError(tt.mockError)
			} else {
				mock.ExpectExec("INSERT INTO feed").
					WithArgs(tt.feedID, tt.feedType, tt.position, tt.feedType, tt.position).
					WillReturnResult(sqlmock.NewResult(1, 1))
			}

			err := store.PatchFeed(ctx, tt.feedID, tt.feedType, tt.position)

			if tt.expectedError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify all expectations were met
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestDeleteFeed(t *testing.T) {
	ctx := context.Background()

	t.Run("begin transaction error", func(t *testing.T) {
		store, mock, cleanup := newMockStore(t)
		defer cleanup()

		mock.ExpectBegin().WillReturnError(sqlmock.ErrCancelled)

		err := store.DeleteFeed(ctx, "feed123")
		if err == nil {
			t.Fatal("expected error but got none")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("feed not found falls back to simple delete", func(t *testing.T) {
		store, mock, cleanup := newMockStore(t)
		defer cleanup()

		mock.ExpectBegin()
		mock.ExpectQuery("SELECT feed_type, position FROM feed").
			WithArgs("nonexistent").
			WillReturnError(sql.ErrNoRows)
		mock.ExpectExec("DELETE FROM feed").
			WithArgs("nonexistent").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit()

		err := store.DeleteFeed(ctx, "nonexistent")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("non-posts feed type does simple delete", func(t *testing.T) {
		store, mock, cleanup := newMockStore(t)
		defer cleanup()

		mock.ExpectBegin()
		mock.ExpectQuery("SELECT feed_type, position FROM feed").
			WithArgs("feed123").
			WillReturnRows(sqlmock.NewRows([]string{"feed_type", "position"}).
				AddRow("banners", 3))
		mock.ExpectExec("DELETE FROM feed").
			WithArgs("feed123").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		err := store.DeleteFeed(ctx, "feed123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("posts feed type with no relations does simple delete", func(t *testing.T) {
		store, mock, cleanup := newMockStore(t)
		defer cleanup()

		mock.ExpectBegin()
		mock.ExpectQuery("SELECT feed_type, position FROM feed").
			WithArgs("feed123").
			WillReturnRows(sqlmock.NewRows([]string{"feed_type", "position"}).
				AddRow("posts", 5))
		mock.ExpectQuery("SELECT feed_id, policies FROM feed_relation").
			WithArgs("feed123").
			WillReturnError(sql.ErrNoRows)
		mock.ExpectExec("DELETE FROM feed").
			WithArgs("feed123").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		err := store.DeleteFeed(ctx, "feed123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("posts feed type promotes replacement from relation", func(t *testing.T) {
		store, mock, cleanup := newMockStore(t)
		defer cleanup()

		mock.ExpectBegin()
		// 1. Get the feed being deleted
		mock.ExpectQuery("SELECT feed_type, position FROM feed").
			WithArgs("source_id").
			WillReturnRows(sqlmock.NewRows([]string{"feed_type", "position"}).
				AddRow("posts", 5))
		// 2. Find a replacement candidate
		mock.ExpectQuery("SELECT feed_id, policies FROM feed_relation").
			WithArgs("source_id").
			WillReturnRows(sqlmock.NewRows([]string{"feed_id", "policies"}).
				AddRow("replacement_id", pq.StringArray{"exposure:1000"}))
		// 3. Delete the selected relation row
		mock.ExpectExec("DELETE FROM feed_relation").
			WithArgs("replacement_id", "source_id").
			WillReturnResult(sqlmock.NewResult(0, 1))
		// 4. Update remaining relations
		mock.ExpectExec("UPDATE feed_relation SET related_feed_id").
			WithArgs("replacement_id", "source_id").
			WillReturnResult(sqlmock.NewResult(0, 2))
		// 5. Delete the original feed
		mock.ExpectExec("DELETE FROM feed").
			WithArgs("source_id").
			WillReturnResult(sqlmock.NewResult(0, 1))
		// 6. Insert the replacement at the same position
		mock.ExpectExec("INSERT INTO feed").
			WithArgs("replacement_id", model.TypePosts, 5, pq.StringArray{"exposure:1000"}).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := store.DeleteFeed(ctx, "source_id")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("posts promotion with empty policies", func(t *testing.T) {
		store, mock, cleanup := newMockStore(t)
		defer cleanup()

		mock.ExpectBegin()
		mock.ExpectQuery("SELECT feed_type, position FROM feed").
			WithArgs("source_id").
			WillReturnRows(sqlmock.NewRows([]string{"feed_type", "position"}).
				AddRow("posts", 0))
		mock.ExpectQuery("SELECT feed_id, policies FROM feed_relation").
			WithArgs("source_id").
			WillReturnRows(sqlmock.NewRows([]string{"feed_id", "policies"}).
				AddRow("replacement_id", pq.StringArray{}))
		mock.ExpectExec("DELETE FROM feed_relation").
			WithArgs("replacement_id", "source_id").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("UPDATE feed_relation SET related_feed_id").
			WithArgs("replacement_id", "source_id").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DELETE FROM feed").
			WithArgs("source_id").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("INSERT INTO feed").
			WithArgs("replacement_id", model.TypePosts, 0, pq.StringArray{}).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := store.DeleteFeed(ctx, "source_id")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("error on delete relation row during promotion", func(t *testing.T) {
		store, mock, cleanup := newMockStore(t)
		defer cleanup()

		mock.ExpectBegin()
		mock.ExpectQuery("SELECT feed_type, position FROM feed").
			WithArgs("source_id").
			WillReturnRows(sqlmock.NewRows([]string{"feed_type", "position"}).
				AddRow("posts", 5))
		mock.ExpectQuery("SELECT feed_id, policies FROM feed_relation").
			WithArgs("source_id").
			WillReturnRows(sqlmock.NewRows([]string{"feed_id", "policies"}).
				AddRow("replacement_id", pq.StringArray{"exposure:1000"}))
		mock.ExpectExec("DELETE FROM feed_relation").
			WillReturnError(sqlmock.ErrCancelled)
		mock.ExpectRollback()

		err := store.DeleteFeed(ctx, "source_id")
		if err == nil {
			t.Fatal("expected error but got none")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("error on update relations during promotion", func(t *testing.T) {
		store, mock, cleanup := newMockStore(t)
		defer cleanup()

		mock.ExpectBegin()
		mock.ExpectQuery("SELECT feed_type, position FROM feed").
			WithArgs("source_id").
			WillReturnRows(sqlmock.NewRows([]string{"feed_type", "position"}).
				AddRow("posts", 5))
		mock.ExpectQuery("SELECT feed_id, policies FROM feed_relation").
			WithArgs("source_id").
			WillReturnRows(sqlmock.NewRows([]string{"feed_id", "policies"}).
				AddRow("replacement_id", pq.StringArray{"exposure:1000"}))
		mock.ExpectExec("DELETE FROM feed_relation").
			WithArgs("replacement_id", "source_id").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("UPDATE feed_relation SET related_feed_id").
			WillReturnError(sqlmock.ErrCancelled)
		mock.ExpectRollback()

		err := store.DeleteFeed(ctx, "source_id")
		if err == nil {
			t.Fatal("expected error but got none")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("error on delete original feed during promotion", func(t *testing.T) {
		store, mock, cleanup := newMockStore(t)
		defer cleanup()

		mock.ExpectBegin()
		mock.ExpectQuery("SELECT feed_type, position FROM feed").
			WithArgs("source_id").
			WillReturnRows(sqlmock.NewRows([]string{"feed_type", "position"}).
				AddRow("posts", 5))
		mock.ExpectQuery("SELECT feed_id, policies FROM feed_relation").
			WithArgs("source_id").
			WillReturnRows(sqlmock.NewRows([]string{"feed_id", "policies"}).
				AddRow("replacement_id", pq.StringArray{"exposure:1000"}))
		mock.ExpectExec("DELETE FROM feed_relation").
			WithArgs("replacement_id", "source_id").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("UPDATE feed_relation SET related_feed_id").
			WithArgs("replacement_id", "source_id").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DELETE FROM feed").
			WillReturnError(sqlmock.ErrCancelled)
		mock.ExpectRollback()

		err := store.DeleteFeed(ctx, "source_id")
		if err == nil {
			t.Fatal("expected error but got none")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("error on insert replacement during promotion", func(t *testing.T) {
		store, mock, cleanup := newMockStore(t)
		defer cleanup()

		mock.ExpectBegin()
		mock.ExpectQuery("SELECT feed_type, position FROM feed").
			WithArgs("source_id").
			WillReturnRows(sqlmock.NewRows([]string{"feed_type", "position"}).
				AddRow("posts", 5))
		mock.ExpectQuery("SELECT feed_id, policies FROM feed_relation").
			WithArgs("source_id").
			WillReturnRows(sqlmock.NewRows([]string{"feed_id", "policies"}).
				AddRow("replacement_id", pq.StringArray{"exposure:1000"}))
		mock.ExpectExec("DELETE FROM feed_relation").
			WithArgs("replacement_id", "source_id").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("UPDATE feed_relation SET related_feed_id").
			WithArgs("replacement_id", "source_id").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DELETE FROM feed").
			WithArgs("source_id").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("INSERT INTO feed").
			WillReturnError(sqlmock.ErrCancelled)
		mock.ExpectRollback()

		err := store.DeleteFeed(ctx, "source_id")
		if err == nil {
			t.Fatal("expected error but got none")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("error on simple delete when feed not found", func(t *testing.T) {
		store, mock, cleanup := newMockStore(t)
		defer cleanup()

		mock.ExpectBegin()
		mock.ExpectQuery("SELECT feed_type, position FROM feed").
			WithArgs("feed123").
			WillReturnError(sql.ErrNoRows)
		mock.ExpectExec("DELETE FROM feed").
			WithArgs("feed123").
			WillReturnError(sqlmock.ErrCancelled)
		mock.ExpectRollback()

		err := store.DeleteFeed(ctx, "feed123")
		if err == nil {
			t.Fatal("expected error but got none")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("error on simple delete for non-posts type", func(t *testing.T) {
		store, mock, cleanup := newMockStore(t)
		defer cleanup()

		mock.ExpectBegin()
		mock.ExpectQuery("SELECT feed_type, position FROM feed").
			WithArgs("feed123").
			WillReturnRows(sqlmock.NewRows([]string{"feed_type", "position"}).
				AddRow("banners", 3))
		mock.ExpectExec("DELETE FROM feed").
			WithArgs("feed123").
			WillReturnError(sqlmock.ErrCancelled)
		mock.ExpectRollback()

		err := store.DeleteFeed(ctx, "feed123")
		if err == nil {
			t.Fatal("expected error but got none")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})

	t.Run("error on simple delete for posts with no relations", func(t *testing.T) {
		store, mock, cleanup := newMockStore(t)
		defer cleanup()

		mock.ExpectBegin()
		mock.ExpectQuery("SELECT feed_type, position FROM feed").
			WithArgs("feed123").
			WillReturnRows(sqlmock.NewRows([]string{"feed_type", "position"}).
				AddRow("posts", 5))
		mock.ExpectQuery("SELECT feed_id, policies FROM feed_relation").
			WithArgs("feed123").
			WillReturnError(sql.ErrNoRows)
		mock.ExpectExec("DELETE FROM feed").
			WithArgs("feed123").
			WillReturnError(sqlmock.ErrCancelled)
		mock.ExpectRollback()

		err := store.DeleteFeed(ctx, "feed123")
		if err == nil {
			t.Fatal("expected error but got none")
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
	})
}

func TestGetPoliciesOrderBy(t *testing.T) {
	ctx := context.Background()
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	// Create rows in reverse order to verify ORDER BY works
	mockRows := sqlmock.NewRows([]string{"feed_id", "feed_type", "position", "policies"}).
		AddRow("feed1", "post", 0, pq.StringArray{}).
		AddRow("feed2", "post", 1, pq.StringArray{}).
		AddRow("feed3", "post", 2, pq.StringArray{})

	// Expect the query and verify ORDER BY clause
	mock.ExpectQuery("SELECT(.+)FROM(.+)feed(.+)ORDER BY(.+)feed.position ASC").
		WillReturnRows(mockRows)

	policies, err := store.GetPolicies(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify positions are in ascending order
	for i := 0; i < len(policies); i++ {
		if policies[i].Position != i {
			t.Errorf("expected position %d at index %d, got %d", i, i, policies[i].Position)
		}
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestFeedChangelogTableCreation(t *testing.T) {
	t.Run("panics on changelog table creation error", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic on changelog table creation error, but did not panic")
			} else {
				panicMsg := r.(string)
				if panicMsg != "failed to create feed_changelog table: canceling query due to user request" {
					t.Errorf("unexpected panic message: %s", panicMsg)
				}
			}
		}()

		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock db: %v", err)
		}
		defer db.Close()

		// Feed table creation succeeds
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS feed").WillReturnResult(sqlmock.NewResult(0, 0))
		// Coldstart table creation succeeds
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS feed_coldstart").WillReturnResult(sqlmock.NewResult(0, 0))
		// Policy format constraint succeeds
		mock.ExpectExec("DO \\$\\$").WillReturnResult(sqlmock.NewResult(0, 0))
		// Feed relation table creation succeeds
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS feed_relation").WillReturnResult(sqlmock.NewResult(0, 0))
		// Changelog table creation fails
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS feed_changelog").WillReturnError(sqlmock.ErrCancelled)

		sqlxDB := sqlx.NewDb(db, "postgres")
		NewFeed(sqlxDB)
	})

	t.Run("panics on changelog trigger creation error", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic on changelog trigger creation error, but did not panic")
			} else {
				panicMsg := r.(string)
				if panicMsg != "failed to create feed_changelog trigger: canceling query due to user request" {
					t.Errorf("unexpected panic message: %s", panicMsg)
				}
			}
		}()

		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock db: %v", err)
		}
		defer db.Close()

		// Feed table creation succeeds
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS feed").WillReturnResult(sqlmock.NewResult(0, 0))
		// Coldstart table creation succeeds
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS feed_coldstart").WillReturnResult(sqlmock.NewResult(0, 0))
		// Policy format constraint succeeds
		mock.ExpectExec("DO \\$\\$").WillReturnResult(sqlmock.NewResult(0, 0))
		// Feed relation table creation succeeds
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS feed_relation").WillReturnResult(sqlmock.NewResult(0, 0))
		// Changelog table creation succeeds
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS feed_changelog").WillReturnResult(sqlmock.NewResult(0, 0))
		// Changelog trigger creation fails
		mock.ExpectExec("DO \\$\\$").WillReturnError(sqlmock.ErrCancelled)

		sqlxDB := sqlx.NewDb(db, "postgres")
		NewFeed(sqlxDB)
	})

	t.Run("changelog table has correct schema", func(t *testing.T) {
		// Verify the expected columns in the changelog table SQL
		expectedColumns := []string{
			"id SERIAL PRIMARY KEY",
			"feed_id uuid NOT NULL",
			"change_type character varying(20) NOT NULL",
			"old_feed_type character varying(20)",
			"new_feed_type character varying(20)",
			"old_position integer",
			"new_position integer",
			"old_policies character varying(200)[]",
			"new_policies character varying(200)[]",
			"changed_at timestamp with time zone NOT NULL DEFAULT NOW()",
		}

		for _, col := range expectedColumns {
			if !contains(createFeedChangelogTableSQL, col) {
				t.Errorf("changelog table SQL missing expected column definition: %s", col)
			}
		}
	})
}

func TestFeedChangelogTriggerLogic(t *testing.T) {
	// These tests document the expected trigger behavior
	// The trigger itself runs at the database level, so we verify the SQL content

	t.Run("trigger handles INSERT operations", func(t *testing.T) {
		expectedContent := []string{
			"IF TG_OP = 'INSERT' THEN",
			"INSERT INTO feed_changelog (feed_id, change_type, new_feed_type, new_position, new_policies)",
			"VALUES (NEW.feed_id, 'INSERT', NEW.feed_type, NEW.position, NEW.policies)",
		}

		for _, content := range expectedContent {
			if !contains(createFeedChangelogTriggerSQL, content) {
				t.Errorf("trigger SQL missing INSERT handling: %s", content)
			}
		}
	})

	t.Run("trigger handles DELETE operations", func(t *testing.T) {
		expectedContent := []string{
			"ELSIF TG_OP = 'DELETE' THEN",
			"INSERT INTO feed_changelog (feed_id, change_type, old_feed_type, old_position, old_policies)",
			"VALUES (OLD.feed_id, 'DELETE', OLD.feed_type, OLD.position, OLD.policies)",
		}

		for _, content := range expectedContent {
			if !contains(createFeedChangelogTriggerSQL, content) {
				t.Errorf("trigger SQL missing DELETE handling: %s", content)
			}
		}
	})

	t.Run("trigger handles UPDATE operations with change detection", func(t *testing.T) {
		expectedContent := []string{
			"ELSIF TG_OP = 'UPDATE' THEN",
			"OLD.feed_type IS DISTINCT FROM NEW.feed_type",
			"OLD.position IS DISTINCT FROM NEW.position",
			"OLD.policies IS DISTINCT FROM NEW.policies",
		}

		for _, content := range expectedContent {
			if !contains(createFeedChangelogTriggerSQL, content) {
				t.Errorf("trigger SQL missing UPDATE handling: %s", content)
			}
		}
	})

	t.Run("trigger detects policy additions", func(t *testing.T) {
		expectedContent := []string{
			"IF cardinality(NEW.policies) > cardinality(OLD.policies) THEN",
			"change_type_val := 'POLICY_ADD'",
		}

		for _, content := range expectedContent {
			if !contains(createFeedChangelogTriggerSQL, content) {
				t.Errorf("trigger SQL missing POLICY_ADD detection: %s", content)
			}
		}
	})

	t.Run("trigger detects policy deletions", func(t *testing.T) {
		expectedContent := []string{
			"ELSIF cardinality(NEW.policies) < cardinality(OLD.policies) THEN",
			"change_type_val := 'POLICY_DELETE'",
		}

		for _, content := range expectedContent {
			if !contains(createFeedChangelogTriggerSQL, content) {
				t.Errorf("trigger SQL missing POLICY_DELETE detection: %s", content)
			}
		}
	})

	t.Run("trigger detects policy modifications", func(t *testing.T) {
		if !contains(createFeedChangelogTriggerSQL, "change_type_val := 'POLICY_MODIFY'") {
			t.Error("trigger SQL missing POLICY_MODIFY detection")
		}
	})

	t.Run("trigger logs generic UPDATE for non-policy changes", func(t *testing.T) {
		if !contains(createFeedChangelogTriggerSQL, "change_type_val := 'UPDATE'") {
			t.Error("trigger SQL missing generic UPDATE change type")
		}
	})

	t.Run("trigger fires after INSERT, UPDATE, DELETE", func(t *testing.T) {
		if !contains(createFeedChangelogTriggerSQL, "AFTER INSERT OR UPDATE OR DELETE ON feed") {
			t.Error("trigger should fire AFTER INSERT OR UPDATE OR DELETE")
		}
	})

	t.Run("trigger executes for each row", func(t *testing.T) {
		if !contains(createFeedChangelogTriggerSQL, "FOR EACH ROW") {
			t.Error("trigger should execute FOR EACH ROW")
		}
	})
}

func TestChangelogTriggerRecreation(t *testing.T) {
	// Verify the trigger uses CREATE OR REPLACE and DROP IF EXISTS
	// to ensure idempotent migrations

	t.Run("trigger function uses CREATE OR REPLACE", func(t *testing.T) {
		if !contains(createFeedChangelogTriggerSQL, "CREATE OR REPLACE FUNCTION log_feed_changes()") {
			t.Error("trigger should use CREATE OR REPLACE FUNCTION for idempotent migrations")
		}
	})

	t.Run("old trigger is dropped before creation", func(t *testing.T) {
		if !contains(createFeedChangelogTriggerSQL, "DROP TRIGGER IF EXISTS feed_changelog_trigger ON feed") {
			t.Error("trigger should drop existing trigger before creating new one")
		}
	})

	t.Run("trigger creation uses correct name", func(t *testing.T) {
		if !contains(createFeedChangelogTriggerSQL, "CREATE TRIGGER feed_changelog_trigger") {
			t.Error("trigger should be named feed_changelog_trigger")
		}
	})
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && searchString(s, substr)))
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
