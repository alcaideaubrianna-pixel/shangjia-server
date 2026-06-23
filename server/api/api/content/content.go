package content

import (
	"context"

	"hotgo/api/api/content/v1"
)

type IContentV1 interface {
	ListProfiles(ctx context.Context, req *v1.ListProfilesReq) (res *v1.ListProfilesRes, err error)
	HomeProfileCards(ctx context.Context, req *v1.HomeProfileCardsReq) (res *v1.HomeProfileCardsRes, err error)
	FilterOptions(ctx context.Context, req *v1.FilterOptionsReq) (res *v1.FilterOptionsRes, err error)
	ViewProfile(ctx context.Context, req *v1.ViewProfileReq) (res *v1.ViewProfileRes, err error)
	ListAnnouncements(ctx context.Context, req *v1.ListAnnouncementsReq) (res *v1.ListAnnouncementsRes, err error)
}
