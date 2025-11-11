package model

import (
	"sort"
)

type Feeds[T Scorable] []Feed[T]

func (f Feeds[T]) Sort() {
	sort.SliceStable(f, func(i, j int) bool {
		return greater(f[i].Data, f[j].Data)
	})
}

func greater[T Scorable](a, b T) bool {
	return a.Score() > b.Score()
}

type Feed[T Scorable] struct {
	ID   string   `json:"id" db:"-"`
	Type FeedType `json:"type" db:"-"`
	Data T        `json:"data" db:"-"`
}

type Scorable interface {
	Feedtype() FeedType
	Score() float64
	GetID() string
}

type FeedType string

const (
	TypePost    FeedType = "post"
	TypeBanners FeedType = "banners"
	TypeChat    FeedType = "chat"
)

type FeedPosition struct {
	FeedID   string   `json:"id" db:"feed_id"`
	FeedType FeedType `json:"type" db:"feed_type"`
	Position int      `json:"position" db:"position"`
	Policy   []string `json:"policy" db:"policy"`
}
