package internal

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	mediaadminv1 "github.com/Muxcore-Media/contracts-media-admin/gen/muxcore/media/admin/v1"
	booksv1 "github.com/Muxcore-Media/media-books/proto/gen/muxcore/books/v1"
)

type mediaAdminServer struct {
	mediaadminv1.UnimplementedMediaAdminServiceServer
	m *Module
}

func (s mediaAdminServer) GetMediaTypeInfo(ctx context.Context, req *mediaadminv1.GetMediaTypeInfoRequest) (*mediaadminv1.GetMediaTypeInfoResponse, error) {
	return s.m.GetMediaTypeInfo(ctx, req)
}

func (s mediaAdminServer) ListItems(ctx context.Context, req *mediaadminv1.ListItemsRequest) (*mediaadminv1.ListItemsResponse, error) {
	return s.m.ListItems(ctx, req)
}

func (s mediaAdminServer) GetItem(ctx context.Context, req *mediaadminv1.GetItemRequest) (*mediaadminv1.GetItemResponse, error) {
	return s.m.GetItem(ctx, req)
}

func (s mediaAdminServer) UpdateMetadata(ctx context.Context, req *mediaadminv1.UpdateMetadataRequest) (*mediaadminv1.UpdateMetadataResponse, error) {
	return s.m.UpdateMetadata(ctx, req)
}

func (s mediaAdminServer) ListArtwork(ctx context.Context, req *mediaadminv1.ListArtworkRequest) (*mediaadminv1.ListArtworkResponse, error) {
	return s.m.ListArtwork(ctx, req)
}

func (s mediaAdminServer) ReplaceArtwork(stream mediaadminv1.MediaAdminService_ReplaceArtworkServer) error {
	return s.m.ReplaceArtwork(stream)
}

func (s mediaAdminServer) DeleteItem(ctx context.Context, req *mediaadminv1.DeleteItemRequest) (*mediaadminv1.DeleteItemResponse, error) {
	return s.m.DeleteItem(ctx, req)
}

func (s mediaAdminServer) RefreshItem(ctx context.Context, req *mediaadminv1.RefreshItemRequest) (*mediaadminv1.RefreshItemResponse, error) {
	return s.m.RefreshItem(ctx, req)
}

func (s mediaAdminServer) SearchIndexers(_ context.Context, _ *mediaadminv1.SearchIndexersRequest) (*mediaadminv1.SearchIndexersResponse, error) {
	return nil, status.Error(codes.Unimplemented, "indexer search not supported for books")
}

func (s mediaAdminServer) ListHistory(ctx context.Context, req *mediaadminv1.ListHistoryRequest) (*mediaadminv1.ListHistoryResponse, error) {
	return s.m.ListHistory(ctx, req)
}

func (s mediaAdminServer) ListMissing(ctx context.Context, req *mediaadminv1.ListMissingRequest) (*mediaadminv1.ListMissingResponse, error) {
	resp, err := (&bookServer{m: s.m}).ListMissing(ctx, &booksv1.ListMissingRequest{
		Page: req.GetPage(), PageSize: req.GetPageSize(), AuthorId: req.GetParentId(),
	})
	if err != nil {
		return nil, err
	}
	items := make([]*mediaadminv1.MissingItem, 0, len(resp.GetItems()))
	for _, it := range resp.GetItems() {
		items = append(items, &mediaadminv1.MissingItem{
			Id: it.GetBookId(), ParentId: it.GetAuthorId(),
			Title: it.GetTitle(), Year: it.GetYear(),
			Metadata: map[string]string{"author_name": it.GetAuthorName()},
		})
	}
	return &mediaadminv1.ListMissingResponse{
		Items: items, Total: resp.GetTotal(), Page: resp.GetPage(), PageSize: resp.GetPageSize(),
	}, nil
}

func (s mediaAdminServer) ListTags(ctx context.Context, _ *mediaadminv1.ListTagsRequest) (*mediaadminv1.ListTagsResponse, error) {
	if s.m.store == nil {
		return nil, status.Error(codes.FailedPrecondition, "store not open")
	}
	tags, err := s.m.store.ListTags(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*mediaadminv1.Tag, 0, len(tags))
	for _, t := range tags {
		out = append(out, &mediaadminv1.Tag{Id: t.ID, Label: t.Label, CreatedAt: t.CreatedAt})
	}
	return &mediaadminv1.ListTagsResponse{Tags: out}, nil
}

func (s mediaAdminServer) CreateTag(ctx context.Context, req *mediaadminv1.CreateTagRequest) (*mediaadminv1.CreateTagResponse, error) {
	if s.m.store == nil {
		return nil, status.Error(codes.FailedPrecondition, "store not open")
	}
	id, err := s.m.store.CreateTag(ctx, req.GetLabel())
	if err != nil {
		return nil, err
	}
	return &mediaadminv1.CreateTagResponse{TagId: id}, nil
}

func (s mediaAdminServer) DeleteTag(ctx context.Context, req *mediaadminv1.DeleteTagRequest) (*mediaadminv1.DeleteTagResponse, error) {
	if s.m.store == nil {
		return nil, status.Error(codes.FailedPrecondition, "store not open")
	}
	if err := s.m.store.DeleteTag(ctx, req.GetTagId()); err != nil {
		return nil, err
	}
	return &mediaadminv1.DeleteTagResponse{}, nil
}

func (s mediaAdminServer) SetItemTags(ctx context.Context, req *mediaadminv1.SetItemTagsRequest) (*mediaadminv1.SetItemTagsResponse, error) {
	if s.m.store == nil {
		return nil, status.Error(codes.FailedPrecondition, "store not open")
	}
	if _, err := mustAuthor(ctx, s.m.store, req.GetItemId()); err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	if err := s.m.store.SetItemTags(ctx, req.GetItemId(), req.GetTagIds()); err != nil {
		return nil, err
	}
	return &mediaadminv1.SetItemTagsResponse{}, nil
}

func (s mediaAdminServer) ListCollections(_ context.Context, _ *mediaadminv1.ListCollectionsRequest) (*mediaadminv1.ListCollectionsResponse, error) {
	return &mediaadminv1.ListCollectionsResponse{}, nil
}

func (s mediaAdminServer) GetCollectionItems(_ context.Context, _ *mediaadminv1.GetCollectionItemsRequest) (*mediaadminv1.GetCollectionItemsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "collections are not supported for books")
}

func (s mediaAdminServer) GetCalendar(_ context.Context, _ *mediaadminv1.GetCalendarRequest) (*mediaadminv1.GetCalendarResponse, error) {
	return nil, status.Error(codes.Unimplemented, "calendar is not supported for books")
}
