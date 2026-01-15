package core

import (
	"context"
	"errors"
	"strconv"
	"testing"
)

func TestPaginatedActivity_FetchesAllPages(t *testing.T) {
	t.Parallel()

	pages := [][]string{
		{"a", "b", "c"},
		{"d", "e", "f"},
		{"g", "h"},
	}

	fetcher := mockFetcher(pages)
	activity := paginatedActivity(fetcher, defaultPaginationConfig())

	result, err := activity(context.Background(), PaginateInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedItems := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	if len(result.Items) != len(expectedItems) {
		t.Errorf("got %d items, want %d", len(result.Items), len(expectedItems))
	}

	for i, item := range result.Items {
		if item != expectedItems[i] {
			t.Errorf("item %d: got %q, want %q", i, item, expectedItems[i])
		}
	}

	if result.PageCount != 3 {
		t.Errorf("got %d pages, want 3", result.PageCount)
	}

	if result.TotalItems != 8 {
		t.Errorf("got %d total items, want 8", result.TotalItems)
	}
}

func TestPaginatedActivity_RespectsMaxPages(t *testing.T) {
	t.Parallel()

	pages := [][]string{
		{"a", "b"},
		{"c", "d"},
		{"e", "f"},
		{"g", "h"},
	}

	fetcher := mockFetcher(pages)
	config := PaginationConfig{MaxPages: 2}
	activity := paginatedActivity(fetcher, config)

	result, err := activity(context.Background(), PaginateInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.PageCount != 2 {
		t.Errorf("got %d pages, want 2", result.PageCount)
	}

	if len(result.Items) != 4 {
		t.Errorf("got %d items, want 4", len(result.Items))
	}

	expectedItems := []string{"a", "b", "c", "d"}
	for i, item := range result.Items {
		if item != expectedItems[i] {
			t.Errorf("item %d: got %q, want %q", i, item, expectedItems[i])
		}
	}
}

func TestPaginatedActivity_HandlesEmptyPages(t *testing.T) {
	t.Parallel()

	fetcher := func(ctx context.Context, cursor string) (PageResult[string], error) {
		return PageResult[string]{
			Items:      nil,
			NextCursor: "",
			HasMore:    false,
		}, nil
	}

	activity := paginatedActivity(fetcher, defaultPaginationConfig())

	result, err := activity(context.Background(), PaginateInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.PageCount != 1 {
		t.Errorf("got %d pages, want 1", result.PageCount)
	}

	if len(result.Items) != 0 {
		t.Errorf("got %d items, want 0", len(result.Items))
	}
}

func TestPaginatedActivity_PropagatesErrors(t *testing.T) {
	t.Parallel()

	fetchErr := errors.New("api error")
	callCount := 0

	fetcher := func(ctx context.Context, cursor string) (PageResult[string], error) {
		callCount++
		if callCount == 2 {
			return PageResult[string]{}, fetchErr
		}
		return PageResult[string]{
			Items:      []string{"item"},
			NextCursor: "next",
			HasMore:    true,
		}, nil
	}

	activity := paginatedActivity(fetcher, defaultPaginationConfig())

	_, err := activity(context.Background(), PaginateInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, fetchErr) {
		t.Errorf("expected wrapped error to contain %v, got %v", fetchErr, err)
	}
}

func TestPaginatedActivity_RespectsContextCancellation(t *testing.T) {
	t.Parallel()

	fetcher := func(ctx context.Context, cursor string) (PageResult[string], error) {
		return PageResult[string]{
			Items:      []string{"item"},
			NextCursor: "next",
			HasMore:    true,
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	activity := paginatedActivity(fetcher, defaultPaginationConfig())

	_, err := activity(ctx, PaginateInput{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestPaginatedActivity_StartsFromCursor(t *testing.T) {
	t.Parallel()

	var startCursor string
	fetcher := func(ctx context.Context, cursor string) (PageResult[string], error) {
		startCursor = cursor
		return PageResult[string]{
			Items:      []string{"item"},
			NextCursor: "",
			HasMore:    false,
		}, nil
	}

	activity := paginatedActivity(fetcher, defaultPaginationConfig())

	_, err := activity(context.Background(), PaginateInput{StartCursor: "page5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if startCursor != "page5" {
		t.Errorf("got start cursor %q, want %q", startCursor, "page5")
	}
}

func TestPaginatedActivity_TracksFinalCursor(t *testing.T) {
	t.Parallel()

	pages := [][]string{
		{"a"},
		{"b"},
		{"c"},
	}
	fetcher := mockFetcher(pages)

	activity := paginatedActivity(fetcher, defaultPaginationConfig())

	result, err := activity(context.Background(), PaginateInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.FinalCursor != "" {
		t.Errorf("got final cursor %q, want empty string", result.FinalCursor)
	}
}

func TestPaginatedActivity_MaxPagesTracksFinalCursor(t *testing.T) {
	t.Parallel()

	pages := [][]string{
		{"a"},
		{"b"},
		{"c"},
		{"d"},
	}
	fetcher := mockFetcher(pages)
	config := PaginationConfig{MaxPages: 2}

	activity := paginatedActivity(fetcher, config)

	result, err := activity(context.Background(), PaginateInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.FinalCursor != "2" {
		t.Errorf("got final cursor %q, want %q", result.FinalCursor, "2")
	}
}

func TestPaginationOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		opts     []PaginationOption
		expected PaginationConfig
	}{
		{
			name:     "default config",
			opts:     nil,
			expected: PaginationConfig{MaxPages: 0, PageSize: 50},
		},
		{
			name:     "with max pages",
			opts:     []PaginationOption{WithMaxPages(10)},
			expected: PaginationConfig{MaxPages: 10, PageSize: 50},
		},
		{
			name:     "with page size",
			opts:     []PaginationOption{WithPageSize(100)},
			expected: PaginationConfig{MaxPages: 0, PageSize: 100},
		},
		{
			name:     "with cursor source",
			opts:     []PaginationOption{WithCursorSource("jira")},
			expected: PaginationConfig{MaxPages: 0, PageSize: 50, CursorSource: "jira"},
		},
		{
			name: "multiple options",
			opts: []PaginationOption{
				WithMaxPages(5),
				WithPageSize(25),
				WithCursorSource("confluence"),
			},
			expected: PaginationConfig{MaxPages: 5, PageSize: 25, CursorSource: "confluence"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := defaultPaginationConfig()
			for _, opt := range tt.opts {
				opt(&config)
			}

			if config.MaxPages != tt.expected.MaxPages {
				t.Errorf("MaxPages: got %d, want %d", config.MaxPages, tt.expected.MaxPages)
			}
			if config.PageSize != tt.expected.PageSize {
				t.Errorf("PageSize: got %d, want %d", config.PageSize, tt.expected.PageSize)
			}
			if config.CursorSource != tt.expected.CursorSource {
				t.Errorf("CursorSource: got %q, want %q", config.CursorSource, tt.expected.CursorSource)
			}
		})
	}
}

func TestPaginate_CreatesNode(t *testing.T) {
	t.Parallel()

	fetcher := func(ctx context.Context, cursor string) (PageResult[string], error) {
		return PageResult[string]{Items: []string{"test"}, HasMore: false}, nil
	}

	node := Paginate("test.paginate", fetcher)

	if node.Name() != "test.paginate" {
		t.Errorf("got name %q, want %q", node.Name(), "test.paginate")
	}
}

func TestPaginateWithConfig_FetchesAllPages(t *testing.T) {
	t.Parallel()

	type testConfig struct {
		Prefix string
	}

	pages := [][]string{
		{"a", "b"},
		{"c", "d"},
	}
	pageIndex := 0

	fetcher := func(ctx context.Context, cfg testConfig, cursor string) (PageResult[string], error) {
		items := make([]string, len(pages[pageIndex]))
		for i, item := range pages[pageIndex] {
			items[i] = cfg.Prefix + item
		}
		pageIndex++
		hasMore := pageIndex < len(pages)
		nextCursor := ""
		if hasMore {
			nextCursor = strconv.Itoa(pageIndex)
		}
		return PageResult[string]{Items: items, NextCursor: nextCursor, HasMore: hasMore}, nil
	}

	node := PaginateWithConfig[string, testConfig]("test.paginate", fetcher)

	if node.Name() != "test.paginate" {
		t.Errorf("got name %q, want %q", node.Name(), "test.paginate")
	}
}

func mockFetcher(pages [][]string) PageFetcher[string] {
	return func(ctx context.Context, cursor string) (PageResult[string], error) {
		pageIndex := 0
		if cursor != "" {
			var err error
			pageIndex, err = strconv.Atoi(cursor)
			if err != nil {
				return PageResult[string]{}, err
			}
		}

		if pageIndex >= len(pages) {
			return PageResult[string]{HasMore: false}, nil
		}

		hasMore := pageIndex+1 < len(pages)
		nextCursor := ""
		if hasMore {
			nextCursor = strconv.Itoa(pageIndex + 1)
		}

		return PageResult[string]{
			Items:      pages[pageIndex],
			NextCursor: nextCursor,
			HasMore:    hasMore,
		}, nil
	}
}
