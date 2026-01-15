package core

import (
	"context"
	"fmt"
)

// PageResult represents a single page of results from a paginated API.
type PageResult[T any] struct {
	// Items are the results from this page.
	Items []T
	// NextCursor is the cursor to fetch the next page. Empty string means no more pages.
	NextCursor string
	// HasMore indicates whether there are more pages to fetch.
	HasMore bool
}

// PageFetcher is the function signature for fetching a single page.
// It receives a cursor (empty string for first page) and returns a PageResult.
type PageFetcher[T any] func(ctx context.Context, cursor string) (PageResult[T], error)

// PaginationConfig configures pagination behavior.
type PaginationConfig struct {
	// MaxPages limits the number of pages to fetch (0 = unlimited).
	MaxPages int
	// PageSize is a hint to the fetcher for items per page.
	PageSize int
	// CursorSource is the key for persisting cursor to FlowState between runs.
	// Empty string means no cursor persistence.
	CursorSource string
}

// PaginationOption configures pagination behavior.
type PaginationOption func(*PaginationConfig)

// WithMaxPages limits the number of pages to fetch.
func WithMaxPages(n int) PaginationOption {
	return func(c *PaginationConfig) {
		c.MaxPages = n
	}
}

// WithPageSize sets the page size hint.
func WithPageSize(n int) PaginationOption {
	return func(c *PaginationConfig) {
		c.PageSize = n
	}
}

// WithCursorSource enables cursor persistence to FlowState.
// The cursor is saved after fetching and restored on next run.
func WithCursorSource(source string) PaginationOption {
	return func(c *PaginationConfig) {
		c.CursorSource = source
	}
}

// defaultPaginationConfig returns default pagination configuration.
func defaultPaginationConfig() PaginationConfig {
	return PaginationConfig{
		MaxPages: 0,  // unlimited
		PageSize: 50, // reasonable default
	}
}

// PaginateInput is the input for a paginated activity.
type PaginateInput struct {
	// StartCursor is the initial cursor position (empty for start).
	StartCursor string
}

// PaginateOutput is the output of a paginated activity.
type PaginateOutput[T any] struct {
	// Items are all collected items across all pages.
	Items []T
	// FinalCursor is the cursor after the last page fetched.
	FinalCursor string
	// PageCount is the number of pages fetched.
	PageCount int
	// TotalItems is the total count of items fetched.
	TotalItems int
}

// paginatedActivity creates an activity function that fetches all pages.
func paginatedActivity[T any](fetcher PageFetcher[T], config PaginationConfig) func(context.Context, PaginateInput) (PaginateOutput[T], error) {
	return func(ctx context.Context, input PaginateInput) (PaginateOutput[T], error) {
		var allItems []T
		cursor := input.StartCursor
		pageCount := 0

		for {
			select {
			case <-ctx.Done():
				return PaginateOutput[T]{}, ctx.Err()
			default:
			}

			result, err := fetcher(ctx, cursor)
			if err != nil {
				return PaginateOutput[T]{}, fmt.Errorf("fetch page %d: %w", pageCount+1, err)
			}

			allItems = append(allItems, result.Items...)
			pageCount++
			cursor = result.NextCursor

			if !result.HasMore || result.NextCursor == "" {
				break
			}

			if config.MaxPages > 0 && pageCount >= config.MaxPages {
				break
			}
		}

		return PaginateOutput[T]{
			Items:       allItems,
			FinalCursor: cursor,
			PageCount:   pageCount,
			TotalItems:  len(allItems),
		}, nil
	}
}

// Paginate creates a Node that fetches all pages from a paginated source.
// The fetcher function is called repeatedly until HasMore is false or MaxPages is reached.
//
// Example usage in a provider:
//
//	func FetchAllIssues(input FetchAllIssuesInput) *core.Node[core.PaginateInput, core.PaginateOutput[Issue]] {
//	    fetcher := func(ctx context.Context, cursor string) (core.PageResult[Issue], error) {
//	        startAt, _ := strconv.Atoi(cursor)
//	        result, err := client.SearchJQL(ctx, input.JQL, startAt, 100)
//	        if err != nil {
//	            return core.PageResult[Issue]{}, err
//	        }
//	        nextCursor := ""
//	        hasMore := startAt+len(result.Issues) < result.Total
//	        if hasMore {
//	            nextCursor = strconv.Itoa(startAt + len(result.Issues))
//	        }
//	        return core.PageResult[Issue]{Items: result.Issues, NextCursor: nextCursor, HasMore: hasMore}, nil
//	    }
//	    return core.Paginate("jira.FetchAllIssues", fetcher)
//	}
func Paginate[T any](name string, fetcher PageFetcher[T], opts ...PaginationOption) *Node[PaginateInput, PaginateOutput[T]] {
	config := defaultPaginationConfig()
	for _, opt := range opts {
		opt(&config)
	}

	activity := paginatedActivity(fetcher, config)
	return NewNode(name, activity, PaginateInput{})
}

// PaginateWithInput creates a Paginate node that also passes through custom input.
// This is useful when the fetcher needs configuration beyond just the cursor.
type PaginateWithInputParams[C any] struct {
	// Config is the custom configuration passed to each page fetch.
	Config C
	// StartCursor is the initial cursor position.
	StartCursor string
}

// PaginateWithInputOutput extends PaginateOutput with the original config.
type PaginateWithInputOutput[T any, C any] struct {
	PaginateOutput[T]
	Config C
}

// ConfiguredPageFetcher is a page fetcher that also receives custom configuration.
type ConfiguredPageFetcher[T any, C any] func(ctx context.Context, config C, cursor string) (PageResult[T], error)

// PaginateWithConfig creates a Node that fetches all pages with custom configuration.
// This is useful when the fetcher needs API credentials, filters, etc.
//
// Example:
//
//	type JiraConfig struct {
//	    BaseURL  string
//	    APIToken string
//	    JQL      string
//	}
//
//	func FetchAllIssues() *core.Node[core.PaginateWithInputParams[JiraConfig], core.PaginateWithInputOutput[Issue, JiraConfig]] {
//	    return core.PaginateWithConfig[Issue, JiraConfig]("jira.FetchAllIssues",
//	        func(ctx context.Context, cfg JiraConfig, cursor string) (core.PageResult[Issue], error) {
//	            // ... fetch logic using cfg
//	        })
//	}
func PaginateWithConfig[T any, C any](
	name string,
	fetcher ConfiguredPageFetcher[T, C],
	opts ...PaginationOption,
) *Node[PaginateWithInputParams[C], PaginateWithInputOutput[T, C]] {
	config := defaultPaginationConfig()
	for _, opt := range opts {
		opt(&config)
	}

	activity := func(ctx context.Context, input PaginateWithInputParams[C]) (PaginateWithInputOutput[T, C], error) {
		var allItems []T
		cursor := input.StartCursor
		pageCount := 0

		for {
			select {
			case <-ctx.Done():
				return PaginateWithInputOutput[T, C]{}, ctx.Err()
			default:
			}

			result, err := fetcher(ctx, input.Config, cursor)
			if err != nil {
				return PaginateWithInputOutput[T, C]{}, fmt.Errorf("fetch page %d: %w", pageCount+1, err)
			}

			allItems = append(allItems, result.Items...)
			pageCount++
			cursor = result.NextCursor

			if !result.HasMore || result.NextCursor == "" {
				break
			}

			if config.MaxPages > 0 && pageCount >= config.MaxPages {
				break
			}
		}

		return PaginateWithInputOutput[T, C]{
			PaginateOutput: PaginateOutput[T]{
				Items:       allItems,
				FinalCursor: cursor,
				PageCount:   pageCount,
				TotalItems:  len(allItems),
			},
			Config: input.Config,
		}, nil
	}

	return NewNode(name, activity, PaginateWithInputParams[C]{})
}
