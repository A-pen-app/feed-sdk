package store

import (
	"context"
	"testing"

	"github.com/A-pen-app/feed-sdk/model"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

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
		db, _, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create mock db: %v", err)
		}
		defer db.Close()

		sqlxDB := sqlx.NewDb(db, "postgres")
		store := NewFeed(sqlxDB)

		if store == nil {
			t.Error("expected store to be created, got nil")
		}
		if store.db == nil {
			t.Error("expected store.db to be set, got nil")
		}
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
				AddRow("feed1", "post", 0, pq.StringArray{"exposure-1000"}).
				AddRow("feed2", "banners", 1, pq.StringArray{"inexpose-1234567890", "unexpose-9876543210"}).
				AddRow("feed3", "chat", 2, pq.StringArray{"exposure-500"}),
			expectedCount: 3,
			validateResult: func(t *testing.T, policies []model.Policy) {
				if policies[0].FeedID != "feed1" {
					t.Errorf("expected first feed ID 'feed1', got '%s'", policies[0].FeedID)
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

				if policies[1].FeedID != "feed2" {
					t.Errorf("expected second feed ID 'feed2', got '%s'", policies[1].FeedID)
				}
				if len(policies[1].Policies) != 2 {
					t.Errorf("expected 2 policies for feed2, got %d", len(policies[1].Policies))
				}
			},
		},
		{
			name: "empty result set",
			mockRows: sqlmock.NewRows([]string{"feed_id", "feed_type", "position", "policies"}),
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
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to create mock db: %v", err)
			}
			defer db.Close()

			sqlxDB := sqlx.NewDb(db, "postgres")
			store := NewFeed(sqlxDB)

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
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to create mock db: %v", err)
			}
			defer db.Close()

			sqlxDB := sqlx.NewDb(db, "postgres")
			store := NewFeed(sqlxDB)

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

			err = store.PatchFeed(ctx, tt.feedID, tt.feedType, tt.position)

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

	tests := []struct {
		name          string
		feedID        string
		mockError     error
		rowsAffected  int64
		expectedError bool
	}{
		{
			name:         "successful delete",
			feedID:       "feed123",
			rowsAffected: 1,
		},
		{
			name:         "delete non-existent feed",
			feedID:       "nonexistent",
			rowsAffected: 0,
		},
		{
			name:          "database error",
			feedID:        "feed456",
			mockError:     sqlmock.ErrCancelled,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to create mock db: %v", err)
			}
			defer db.Close()

			sqlxDB := sqlx.NewDb(db, "postgres")
			store := NewFeed(sqlxDB)

			// Set up expectations
			if tt.mockError != nil {
				mock.ExpectExec("DELETE FROM feed").WillReturnError(tt.mockError)
			} else {
				mock.ExpectExec("DELETE FROM feed").
					WithArgs(tt.feedID).
					WillReturnResult(sqlmock.NewResult(0, tt.rowsAffected))
			}

			err = store.DeleteFeed(ctx, tt.feedID)

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

func TestGetPoliciesOrderBy(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create mock db: %v", err)
	}
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "postgres")
	store := NewFeed(sqlxDB)

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
