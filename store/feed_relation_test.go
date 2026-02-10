package store

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAddRelation(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		feedID        string
		relatedFeedID string
		mockError     error
		expectedError bool
	}{
		{
			name:          "successful insert",
			feedID:        "feed123",
			relatedFeedID: "feed456",
		},
		{
			name:          "insert with same IDs (self-relation)",
			feedID:        "feed123",
			relatedFeedID: "feed123",
		},
		{
			name:          "database error",
			feedID:        "feed789",
			relatedFeedID: "feed012",
			mockError:     sqlmock.ErrCancelled,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, mock, cleanup := newMockStore(t)
			defer cleanup()

			if tt.mockError != nil {
				mock.ExpectExec("INSERT INTO feed_relation").WillReturnError(tt.mockError)
			} else {
				mock.ExpectExec("INSERT INTO feed_relation").
					WithArgs(tt.feedID, tt.relatedFeedID).
					WillReturnResult(sqlmock.NewResult(1, 1))
			}

			err := store.AddRelation(ctx, tt.feedID, tt.relatedFeedID)

			if tt.expectedError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestRemoveRelation(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		feedID        string
		relatedFeedID string
		rowsAffected  int64
		mockError     error
		expectedError bool
	}{
		{
			name:          "successful delete",
			feedID:        "feed123",
			relatedFeedID: "feed456",
			rowsAffected:  1,
		},
		{
			name:          "delete non-existent relation",
			feedID:        "nonexistent1",
			relatedFeedID: "nonexistent2",
			rowsAffected:  0,
		},
		{
			name:          "database error",
			feedID:        "feed789",
			relatedFeedID: "feed012",
			mockError:     sqlmock.ErrCancelled,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, mock, cleanup := newMockStore(t)
			defer cleanup()

			if tt.mockError != nil {
				mock.ExpectExec("DELETE FROM feed_relation").WillReturnError(tt.mockError)
			} else {
				mock.ExpectExec("DELETE FROM feed_relation").
					WithArgs(tt.feedID, tt.relatedFeedID).
					WillReturnResult(sqlmock.NewResult(0, tt.rowsAffected))
			}

			err := store.RemoveRelation(ctx, tt.feedID, tt.relatedFeedID)

			if tt.expectedError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestGetRelatedFeedsStore(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		feedID        string
		mockRows      *sqlmock.Rows
		mockError     error
		expectedIDs   []string
		expectedError bool
	}{
		{
			name:   "successful query with multiple relations",
			feedID: "feed123",
			mockRows: sqlmock.NewRows([]string{"related_feed_id"}).
				AddRow("feed456").
				AddRow("feed789").
				AddRow("feed012"),
			expectedIDs: []string{"feed456", "feed789", "feed012"},
		},
		{
			name:        "empty result set",
			feedID:      "feed123",
			mockRows:    sqlmock.NewRows([]string{"related_feed_id"}),
			expectedIDs: []string{},
		},
		{
			name:          "database error",
			feedID:        "feed123",
			mockError:     sqlmock.ErrCancelled,
			expectedError: true,
		},
		{
			name:   "single relation",
			feedID: "feed123",
			mockRows: sqlmock.NewRows([]string{"related_feed_id"}).
				AddRow("feed456"),
			expectedIDs: []string{"feed456"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, mock, cleanup := newMockStore(t)
			defer cleanup()

			if tt.mockError != nil {
				mock.ExpectQuery("SELECT related_feed_id FROM feed_relation").
					WithArgs(tt.feedID).
					WillReturnError(tt.mockError)
			} else {
				mock.ExpectQuery("SELECT related_feed_id FROM feed_relation").
					WithArgs(tt.feedID).
					WillReturnRows(tt.mockRows)
			}

			relatedIDs, err := store.GetRelatedFeeds(ctx, tt.feedID)

			if tt.expectedError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(relatedIDs) != len(tt.expectedIDs) {
				t.Fatalf("expected %d related IDs, got %d", len(tt.expectedIDs), len(relatedIDs))
			}

			for i, expectedID := range tt.expectedIDs {
				if relatedIDs[i] != expectedID {
					t.Errorf("at index %d: expected ID %s, got %s", i, expectedID, relatedIDs[i])
				}
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}
