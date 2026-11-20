package server

import (
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/service"
)

// The list answers: every list is an object with its items, so a page token can join it later and gRPC has a message to return.
type (
	ListingVersions struct {
		Items []port.ListingVersion `json:"items"`
	}
	Listings struct {
		Items []port.Listing `json:"items"`
	}
	CatalogRules struct {
		Items []port.CatalogRule `json:"items"`
	}
	Offers struct {
		Items []port.Offer `json:"items"`
	}
	Grants struct {
		Items []port.Grant `json:"items"`
	}
	ConnectorViews struct {
		Items []service.ConnectorView `json:"items"`
	}
	Functions struct {
		Items []port.Function `json:"items"`
	}
	AcceptResults struct {
		Items []service.AcceptResult `json:"items"`
	}
	Overlays struct {
		Items []port.Overlay `json:"items"`
	}
	Bindings struct {
		Items []port.Binding `json:"items"`
	}
	Approvals struct {
		Items []port.Approval `json:"items"`
	}
	Pauses struct {
		Items []port.Pause `json:"items"`
	}
	Credentials struct {
		Items []port.Credential `json:"items"`
	}
	Roles struct {
		Items []port.RoleAssignment `json:"items"`
	}
	Calls struct {
		Items []port.Call `json:"items"`
	}
	Sources struct {
		Items []service.SourceStatus `json:"items"`
	}
	GroupMembers struct {
		Items []port.GroupMember `json:"items"`
	}
)

func items[W ~struct {
	Items []T `json:"items"`
}, T any](rows []T, err error) (W, error) {
	return W{Items: rows}, err
}
