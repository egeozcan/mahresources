package models

import "time"

// TimelineBucket represents a single time bucket in the timeline response.
// It holds the count of entities created and updated within the bucket's time range.
type TimelineBucket struct {
	Label   string    `json:"label"`
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Created int64     `json:"created"`
	Updated int64     `json:"updated"`
}

// TimelineHasMore indicates whether there is additional data beyond the
// visible timeline window in either direction.
type TimelineHasMore struct {
	Left  bool `json:"left"`
	Right bool `json:"right"`
}

// TimelineResponse is the API response for timeline endpoints.
type TimelineResponse struct {
	Buckets []TimelineBucket `json:"buckets"`
	HasMore TimelineHasMore  `json:"hasMore"`
}
