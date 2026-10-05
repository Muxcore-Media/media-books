package internal

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	mediaadminv1 "github.com/Muxcore-Media/contracts-media-admin/gen/muxcore/media/admin/v1"
)

// clampInt32 converts n to int32, saturating at the int32 bounds.
func clampInt32(n int) int32 {
	if n > math.MaxInt32 {
		return math.MaxInt32
	}
	if n < math.MinInt32 {
		return math.MinInt32
	}
	return int32(n) //nolint:gosec // bounds checked above
}

func (m *Module) GetMediaTypeInfo(_ context.Context, _ *mediaadminv1.GetMediaTypeInfoRequest) (*mediaadminv1.GetMediaTypeInfoResponse, error) {
	return &mediaadminv1.GetMediaTypeInfoResponse{
		DisplayName: "Books",
		Icon:        "📚",
		FilterFields: []*mediaadminv1.FilterField{
			{Key: "year", Label: "Year", Type: mediaadminv1.FilterFieldType_FILTER_FIELD_TYPE_NUMBER},
			{Key: "monitored", Label: "Monitored", Type: mediaadminv1.FilterFieldType_FILTER_FIELD_TYPE_SELECT, Options: []string{"true", "false"}},
		},
		Features: []mediaadminv1.Feature{mediaadminv1.Feature_FEATURE_MISSING},
	}, nil
}

func (m *Module) ListItems(ctx context.Context, req *mediaadminv1.ListItemsRequest) (*mediaadminv1.ListItemsResponse, error) {
	if m.store == nil {
		return nil, fmt.Errorf("not initialized")
	}
	page := int(req.GetPage())
	if page < 1 {
		page = 1
	}
	pageSize := int(req.GetPageSize())
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	authors, err := m.store.ListAuthors(ctx, req.GetSearch())
	if err != nil {
		return nil, err
	}
	total := len(authors)
	offset := (page - 1) * pageSize
	if offset >= total {
		return &mediaadminv1.ListItemsResponse{
			Items: []*mediaadminv1.MediaItem{}, Total: clampInt32(total),
			Page: clampInt32(page), PageSize: clampInt32(pageSize),
		}, nil
	}
	end := offset + pageSize
	if end > total {
		end = total
	}
	out := make([]*mediaadminv1.MediaItem, 0, end-offset)
	for _, a := range authors[offset:end] {
		out = append(out, m.authorToMediaItem(ctx, a))
	}
	return &mediaadminv1.ListItemsResponse{
		Items: out, Total: clampInt32(total), Page: clampInt32(page), PageSize: clampInt32(pageSize),
	}, nil
}

func (m *Module) GetItem(ctx context.Context, req *mediaadminv1.GetItemRequest) (*mediaadminv1.GetItemResponse, error) {
	a, err := m.store.GetAuthor(ctx, req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return &mediaadminv1.GetItemResponse{Item: m.authorToMediaItem(ctx, a)}, nil
}

func (m *Module) UpdateMetadata(ctx context.Context, req *mediaadminv1.UpdateMetadataRequest) (*mediaadminv1.UpdateMetadataResponse, error) {
	fields := map[string]any{}
	if req.GetTitle() != "" {
		fields["name"] = req.GetTitle()
	}
	if req.GetMetadata() != nil {
		if v := req.GetMetadata()["goodreads_id"]; v != "" {
			fields["goodreads_id"] = v
		}
		if v := req.GetMetadata()["path"]; v != "" {
			fields["path"] = v
		}
		if v := req.GetMetadata()["monitored"]; v != "" {
			fields["monitored"] = v == "true" || v == "1"
		}
	}
	a, err := m.store.UpdateAuthor(ctx, req.GetId(), fields)
	if err != nil {
		return nil, err
	}
	return &mediaadminv1.UpdateMetadataResponse{Item: m.authorToMediaItem(ctx, a)}, nil
}

func (m *Module) ListArtwork(ctx context.Context, req *mediaadminv1.ListArtworkRequest) (*mediaadminv1.ListArtworkResponse, error) {
	a, err := mustAuthor(ctx, m.store, req.GetId())
	if err != nil {
		return nil, err
	}
	var artwork []*mediaadminv1.ArtworkInfo
	if url := m.servableArtworkURL(a.PosterURL); url != "" {
		artwork = append(artwork, &mediaadminv1.ArtworkInfo{
			Id: a.ID + "_poster", ItemId: a.ID,
			Type: mediaadminv1.ArtworkType_ARTWORK_TYPE_POSTER, Url: url,
		})
	}
	return &mediaadminv1.ListArtworkResponse{Artwork: artwork}, nil
}

func (m *Module) DeleteItem(ctx context.Context, req *mediaadminv1.DeleteItemRequest) (*mediaadminv1.DeleteItemResponse, error) {
	if err := m.store.RemoveAuthorFiles(ctx, req.GetId(), m.libraryRoot(), req.GetDeleteFiles()); err != nil {
		return nil, err
	}
	return &mediaadminv1.DeleteItemResponse{}, nil
}

func (m *Module) RefreshItem(ctx context.Context, req *mediaadminv1.RefreshItemRequest) (*mediaadminv1.RefreshItemResponse, error) {
	if _, err := m.ScanLibrary(ctx); err != nil {
		return nil, err
	}
	item, err := m.GetItem(ctx, &mediaadminv1.GetItemRequest{Id: req.GetId()})
	if err != nil {
		return nil, err
	}
	return &mediaadminv1.RefreshItemResponse{Item: item.GetItem()}, nil
}

func (m *Module) authorToMediaItem(ctx context.Context, a *Author) *mediaadminv1.MediaItem {
	meta := map[string]string{
		"goodreads_id": a.GoodreadsID,
		"monitored":    strconv.FormatBool(a.Monitored),
		"path":         a.Path,
	}
	books, _ := m.store.ListBooks(ctx, a.ID)
	meta["book_count"] = strconv.Itoa(len(books))
	return &mediaadminv1.MediaItem{
		Id: a.ID, Title: a.Name, Metadata: meta,
	}
}
