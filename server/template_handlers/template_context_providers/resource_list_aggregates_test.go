package template_context_providers

import (
	"errors"
	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/mrql"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// overlapResourceContext records how many list queries are in flight at once.
// Each query waits until `want` queries have arrived (or a deadline passes), so
// a provider that runs them one after another peaks at 1 and one that runs them
// together peaks at `want`.
type overlapResourceContext struct {
	ResourcePageContext

	want     int32
	inFlight atomic.Int32
	peak     atomic.Int32
	arrived  atomic.Int32
	once     sync.Once
	all      chan struct{}

	countErr  error
	tagsPanic any
}

func newOverlapResourceContext(want int32) *overlapResourceContext {
	return &overlapResourceContext{want: want, all: make(chan struct{})}
}

func (c *overlapResourceContext) enter() func() {
	n := c.inFlight.Add(1)
	for {
		p := c.peak.Load()
		if n <= p || c.peak.CompareAndSwap(p, n) {
			break
		}
	}
	if c.arrived.Add(1) >= c.want {
		c.once.Do(func() { close(c.all) })
	}
	select {
	case <-c.all:
	case <-time.After(300 * time.Millisecond):
	}
	return func() { c.inFlight.Add(-1) }
}

func (c *overlapResourceContext) GetResources(offset, maxResults int, query *query_models.ResourceSearchQuery) ([]models.Resource, error) {
	defer c.enter()()
	return []models.Resource{{ID: 7}}, nil
}

func (c *overlapResourceContext) GetResourceCount(query *query_models.ResourceSearchQuery) (int64, error) {
	defer c.enter()()
	return 1, c.countErr
}

func (c *overlapResourceContext) GetPopularResourceTags(query *query_models.ResourceSearchQuery) ([]application_context.PopularTag, error) {
	defer c.enter()()
	if c.tagsPanic != nil {
		panic(c.tagsPanic)
	}
	return []application_context.PopularTag{{Id: 3, Name: "t", Count: 1}}, nil
}

func (c *overlapResourceContext) CheckMRQLFilter(entity mrql.EntityType, expr string) *application_context.MRQLFilterError {
	return nil
}

func (c *overlapResourceContext) GetTagsWithIds(ids *[]uint, limit int) ([]models.Tag, error) {
	return nil, nil
}

func (c *overlapResourceContext) GetNotesWithIds(ids *[]uint) ([]*models.Note, error) {
	return nil, nil
}

func (c *overlapResourceContext) GetGroupsWithIds(ids *[]uint) ([]*models.Group, error) {
	return nil, nil
}

func TestResourceListRunsListQueriesTogether(t *testing.T) {
	c := newOverlapResourceContext(3)
	ctx := ResourceListContextProvider(c)(httptest.NewRequest("GET", "/resources", nil))

	if got := c.peak.Load(); got != 3 {
		t.Fatalf("resources, count and popular tags should run together; peak in flight = %d, want 3", got)
	}
	if ctx["totalCount"] != int64(1) {
		t.Errorf("totalCount = %v, want 1", ctx["totalCount"])
	}
	if res, _ := ctx["resources"].([]models.Resource); len(res) != 1 || res[0].ID != 7 {
		t.Errorf("resources = %v, want the one fake resource", ctx["resources"])
	}
	if tags, _ := ctx["popularTags"].([]application_context.PopularTag); len(tags) != 1 {
		t.Errorf("popularTags = %v, want the one fake tag", ctx["popularTags"])
	}
}

func TestResourceListReportsAQueryErrorAfterRunningTogether(t *testing.T) {
	c := newOverlapResourceContext(3)
	c.countErr = errors.New("count failed")
	ctx := ResourceListContextProvider(c)(httptest.NewRequest("GET", "/resources", nil))

	if ctx["errorMessage"] != "count failed" {
		t.Fatalf("errorMessage = %v, want the count error", ctx["errorMessage"])
	}
	if _, ok := ctx["resources"]; ok {
		t.Error("a failed list must not render resources")
	}
}

func TestResourceTimelineRunsAggregatesTogether(t *testing.T) {
	c := newOverlapResourceContext(2)
	ctx := ResourceTimelineContextProvider(c)(httptest.NewRequest("GET", "/resources/timeline", nil))

	if got := c.peak.Load(); got != 2 {
		t.Fatalf("count and popular tags should run together; peak in flight = %d, want 2", got)
	}
	if ctx["totalCount"] != int64(1) {
		t.Errorf("totalCount = %v, want 1", ctx["totalCount"])
	}
}

func TestResourceListRaisesAQueryPanicOnTheCaller(t *testing.T) {
	c := newOverlapResourceContext(3)
	c.tagsPanic = "tags exploded"

	defer func() {
		if r := recover(); r != "tags exploded" {
			t.Fatalf("recovered %v, want the query's panic raised on the request goroutine", r)
		}
	}()
	ResourceListContextProvider(c)(httptest.NewRequest("GET", "/resources", nil))
	t.Fatal("the provider returned instead of panicking")
}
