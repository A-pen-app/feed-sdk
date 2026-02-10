package store

import (
	"context"
	"testing"

	"github.com/A-pen-app/feed-sdk/model"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestAddRelation(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		feedID        string
		relatedFeedID string
		feedType      string
		position      int
		mockError     error
		expectedError bool
	}{
		{
			name:          "successful insert",
			feedID:        "feed123",
			relatedFeedID: "feed456",
			feedType:      "post",
			position:      0,
		},
		{
			name:          "insert with same IDs (self-relation)",
			feedID:        "feed123",
			relatedFeedID: "feed123",
			feedType:      "banners",
			position:      1,
		},
		{
			name:          "database error",
			feedID:        "feed789",
			relatedFeedID: "feed012",
			feedType:      "post",
			position:      2,
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
					WithArgs(tt.feedID, tt.relatedFeedID, tt.feedType, tt.position, tt.feedType, tt.position).
					WillReturnResult(sqlmock.NewResult(1, 1))
			}

			err := store.AddRelation(ctx, tt.feedID, tt.relatedFeedID, model.FeedType(tt.feedType), tt.position)

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
		mockRows      *sqlmock.Rows
		mockError     error
		expectedMap   map[string][]string
		expectedError bool
	}{
		{
			name: "successful query with multiple relations",
			mockRows: sqlmock.NewRows([]string{"feed_id", "related_feed_id"}).
				AddRow("feed123", "feed456").
				AddRow("feed123", "feed789").
				AddRow("feed123", "feed012").
				AddRow("feed456", "feed123"),
			expectedMap: map[string][]string{
				"feed123": {"feed456", "feed789", "feed012"},
				"feed456": {"feed123"},
			},
		},
		{
			name:        "empty result set",
			mockRows:    sqlmock.NewRows([]string{"feed_id", "related_feed_id"}),
			expectedMap: map[string][]string{},
		},
		{
			name:          "database error",
			mockError:     sqlmock.ErrCancelled,
			expectedError: true,
		},
		{
			name: "single relation",
			mockRows: sqlmock.NewRows([]string{"feed_id", "related_feed_id"}).
				AddRow("feed123", "feed456"),
			expectedMap: map[string][]string{
				"feed123": {"feed456"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, mock, cleanup := newMockStore(t)
			defer cleanup()

			if tt.mockError != nil {
				mock.ExpectQuery("SELECT feed_id, related_feed_id FROM feed_relation").
					WillReturnError(tt.mockError)
			} else {
				mock.ExpectQuery("SELECT feed_id, related_feed_id FROM feed_relation").
					WillReturnRows(tt.mockRows)
			}

			relatedMap, err := store.GetRelatedFeeds(ctx)

			if tt.expectedError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(relatedMap) != len(tt.expectedMap) {
				t.Fatalf("expected %d keys, got %d", len(tt.expectedMap), len(relatedMap))
			}

			for feedID, expectedIDs := range tt.expectedMap {
				actualIDs, exists := relatedMap[feedID]
				if !exists {
					t.Errorf("expected key %s not found in result", feedID)
					continue
				}
				if len(actualIDs) != len(expectedIDs) {
					t.Errorf("for key %s: expected %d IDs, got %d", feedID, len(expectedIDs), len(actualIDs))
					continue
				}
				for i, expectedID := range expectedIDs {
					if actualIDs[i] != expectedID {
						t.Errorf("for key %s at index %d: expected ID %s, got %s", feedID, i, expectedID, actualIDs[i])
					}
				}
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}
