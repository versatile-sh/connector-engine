package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/agents-control/plugkit/discovery"

	"github.com/versatile-sh/connector-engine/catalog"
	corecatalog "github.com/versatile-sh/connector-engine/core/catalog"
	"github.com/versatile-sh/connector-engine/core/evaluate"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/service"
)

type none = struct{}

func done(err error) (none, error) { return none{}, err }

func lintCapture(e *Env, on bool) service.CaptureFunc {
	if !on {
		return nil
	}
	return e.Svc.Capture
}

func eventType(t catalog.Type) (EventType, error) {
	out := EventType{Number: t.Number, Name: t.Name, Consumed: t.Consumed}
	return out, json.Unmarshal(t.Schema, &out.Schema)
}

func eventTypes(_ context.Context, _ *Env, _ port.Principal, _ none) (EventTypes, error) {
	all, err := catalog.All()
	out := EventTypes{Types: make([]EventType, len(all))}
	for i, t := range all {
		if out.Types[i], err = eventType(t); err != nil {
			return out, err
		}
	}
	return out, err
}

func oneEventType(_ context.Context, _ *Env, _ port.Principal, in EventTypeRequest) (EventType, error) {
	t, ok, err := catalog.Get(in.Type)
	if err != nil || !ok {
		return EventType{}, orNotFound(err, "event type %s", in.Type)
	}
	return eventType(t)
}

func orNotFound(err error, format string, args ...any) error {
	if err != nil {
		return err
	}
	return port.Errorf(port.CodeNotFound, format, args...)
}

func agentFunctions(ctx context.Context, e *Env, p port.Principal, in ListAgentFunctionsRequest) (AgentFunctions, error) {
	list, err := e.Svc.ListAgentFunctions(ctx, p, in.subject(p))
	out := AgentFunctions{Functions: make([]AgentFunction, len(list.Functions)), ApprovalTools: list.ApprovalTools, Exposure: list.Exposure}
	for i, f := range list.Functions {
		out.Functions[i] = AgentFunction{Name: f.Name, Connector: f.Connector, Function: f.Function, Mode: f.Mode, OverlayHash: f.OverlayHash}
	}
	return out, err
}

func changeStatus(status port.ListingStatus) func(context.Context, *Env, port.Principal, ListingStatusRequest) (service.StatusChange, error) {
	return func(ctx context.Context, e *Env, p port.Principal, in ListingStatusRequest) (service.StatusChange, error) {
		return e.Svc.ChangeListingStatus(ctx, p, in.listing(), in.Version, status, in.Reason)
	}
}

func setConnectorEnabled(on bool) func(context.Context, *Env, port.Principal, ConnectorRequest) (port.Connector, error) {
	return func(ctx context.Context, e *Env, p port.Principal, in ConnectorRequest) (port.Connector, error) {
		return e.Svc.SetConnectorEnabled(ctx, p, in.ID, on)
	}
}

func setFunctionDisabled(off bool) func(context.Context, *Env, port.Principal, FunctionRequest) (port.Function, error) {
	return func(ctx context.Context, e *Env, p port.Principal, in FunctionRequest) (port.Function, error) {
		return e.Svc.SetFunctionDisabled(ctx, p, in.ID, in.Name, off)
	}
}

func (r PutBindingRequest) binding() port.Binding {
	return port.Binding{Connector: r.ID, Agent: r.Agent, AgentOwner: r.AgentOwner, CredentialPolicy: r.CredentialPolicy, AskRequester: r.AskRequester, Approvers: r.Approvers}
}

var (
	get    = http.MethodGet
	post   = http.MethodPost
	put    = http.MethodPut
	del    = http.MethodDelete
	ok200  = http.StatusOK
	accept = http.StatusAccepted
	gone   = http.StatusNoContent
)

var operations = []Operation{
	op("listListings", get, "/v1/listings", "ListListings", ok200, func(ctx context.Context, e *Env, p port.Principal, in ListListingsRequest) (ListingVersions, error) {
		return items[ListingVersions](e.Svc.ListListings(ctx, p, in.Workspace, service.ListingFilter{Source: in.Source, Trust: in.TrustLevel, Kind: in.Kind, Query: in.Q}))
	}),
	op("getListing", get, "/v1/listings/version", "GetListing", ok200, func(ctx context.Context, e *Env, p port.Principal, in GetListingRequest) (port.ListingVersion, error) {
		return e.Svc.GetListing(ctx, p, in.Workspace, in.version())
	}),
	op("listOfferedListings", get, "/v1/catalog/offered", "ListOfferedListings", ok200, func(ctx context.Context, e *Env, p port.Principal, _ none) (Listings, error) {
		return items[Listings](e.Svc.ListOfferedListings(ctx, p))
	}),
	op("getCatalogPolicy", get, "/v1/catalog/policies/{workspace}", "GetCatalogPolicy", ok200, func(ctx context.Context, e *Env, p port.Principal, in WorkspaceRequest) (port.CatalogPolicy, error) {
		return e.Svc.GetCatalogPolicy(ctx, p, in.Workspace)
	}),
	op("putCatalogPolicy", put, "/v1/catalog/policies/{workspace}", "PutCatalogPolicy", ok200, func(ctx context.Context, e *Env, p port.Principal, in PutCatalogPolicyRequest) (port.CatalogPolicy, error) {
		return e.Svc.PutCatalogPolicy(ctx, p, in.Subject, in.Workspace, in.Default)
	}),
	op("listCatalogRules", get, "/v1/catalog/rules", "ListCatalogRules", ok200, func(ctx context.Context, e *Env, p port.Principal, _ none) (CatalogRules, error) {
		return items[CatalogRules](e.Svc.ListCatalogRules(ctx, p))
	}),
	op("putCatalogRule", put, "/v1/catalog/rules", "PutCatalogRule", ok200, func(ctx context.Context, e *Env, p port.Principal, in port.CatalogRule) (port.CatalogRule, error) {
		return e.Svc.PutCatalogRule(ctx, p, in)
	}),
	op("deleteCatalogRule", del, "/v1/catalog/rules/{id}", "DeleteCatalogRule", gone, func(ctx context.Context, e *Env, p port.Principal, in RuleRequest) (none, error) {
		return done(e.Svc.DeleteCatalogRule(ctx, p, in.ID))
	}),
	op("explainVisibility", post, "/v1/catalog:explain", "ExplainVisibility", ok200, func(ctx context.Context, e *Env, p port.Principal, in ExplainVisibilityRequest) (corecatalog.Answer, error) {
		return e.Svc.ExplainVisibility(ctx, p, in.Workspace, in.Person, port.ListingRef{Owner: in.Owner, Name: in.Name})
	}),
	op("listOffers", get, "/v1/tenants/{tenant}/offers", "ListOffers", ok200, func(ctx context.Context, e *Env, p port.Principal, in TenantRequest) (Offers, error) {
		return items[Offers](e.Svc.ListOffers(ctx, p, in.Tenant))
	}),
	op("putOffer", put, "/v1/tenants/{tenant}/offers", "PutOffer", accept, func(ctx context.Context, e *Env, p port.Principal, in OfferRequest) (port.Operation, error) {
		return e.Svc.PutOffer(ctx, p, port.Offer{Tenant: in.Tenant, TargetKind: in.TargetKind, Target: in.Target, Withheld: in.Withheld})
	}),
	op("createListing", post, "/v1/listings", "CreateListing", http.StatusCreated, func(ctx context.Context, e *Env, p port.Principal, in service.CreateListingInput) (port.ListingVersion, error) {
		return e.Svc.CreateListing(ctx, p, in)
	}),
	op("quarantineListing", post, "/v1/listings:quarantine", "QuarantineListing", ok200, changeStatus(port.ListingQuarantined)),
	op("deprecateListing", post, "/v1/listings:deprecate", "DeprecateListing", ok200, changeStatus(port.ListingDeprecated)),
	op("withdrawListing", post, "/v1/listings:withdraw", "WithdrawListing", ok200, changeStatus(port.ListingDeleted)),
	op("restoreListing", post, "/v1/listings:restore", "RestoreListing", ok200, changeStatus(port.ListingActive)),
	op("setListingStarter", put, "/v1/listings/starter", "SetListingStarter", ok200, func(ctx context.Context, e *Env, p port.Principal, in StarterRequest) (port.Listing, error) {
		return e.Svc.SetListingStarter(ctx, p, port.ListingRef{Owner: in.Owner, Name: in.Name}, in.Starter)
	}),
	op("createTenant", put, "/v1/tenants/{tenant}", "CreateTenant", ok200, func(ctx context.Context, e *Env, p port.Principal, in TenantRequest) (port.Tenant, error) {
		return e.Svc.CreateTenant(ctx, p, in.Tenant)
	}),
	op("removeTenant", del, "/v1/tenants/{tenant}", "RemoveTenant", accept, func(ctx context.Context, e *Env, p port.Principal, in TenantRequest) (port.Operation, error) {
		return e.Svc.RemoveTenant(ctx, p, in.Tenant)
	}),
	op("lintListing", post, "/v1/listings:lint", "LintListing", ok200, func(ctx context.Context, e *Env, p port.Principal, in LintListingRequest) (service.LintResult, error) {
		return e.Svc.LintListing(ctx, p, in.Manifest, lintCapture(e, in.Capture))
	}),
	op("reviewListing", post, "/v1/listings:review", "ReviewListing", ok200, func(ctx context.Context, e *Env, p port.Principal, in ReviewListingRequest) (port.ListingVersion, error) {
		return e.Svc.ReviewListing(ctx, p, in.version(), in.ReviewInput)
	}),
	op("syncSource", post, "/v1/sources/{source}:sync", "SyncSource", accept, func(ctx context.Context, e *Env, p port.Principal, in SourceRequest) (port.Operation, error) {
		return e.Svc.SyncSource(ctx, p, in.Source)
	}),
	op("installConnector", post, "/v1/connectors", "InstallConnector", accept, func(ctx context.Context, e *Env, p port.Principal, in service.InstallInput) (service.ConnectorView, error) {
		return e.Svc.InstallConnector(ctx, p, in)
	}),
	op("enableConnector", post, "/v1/connectors/{id}:enable", "EnableConnector", ok200, setConnectorEnabled(true)),
	op("disableConnector", post, "/v1/connectors/{id}:disable", "DisableConnector", ok200, setConnectorEnabled(false)),
	op("getConnector", get, "/v1/connectors/{id}", "GetConnector", ok200, func(ctx context.Context, e *Env, p port.Principal, in ConnectorRequest) (service.ConnectorView, error) {
		return e.Svc.GetConnector(ctx, p, in.ID)
	}),
	op("listConnectors", get, "/v1/connectors", "ListConnectors", ok200, func(ctx context.Context, e *Env, p port.Principal, in WorkspaceRequest) (ConnectorViews, error) {
		return items[ConnectorViews](e.Svc.ListConnectors(ctx, p, in.Workspace))
	}),
	op("updateConnector", post, "/v1/connectors/{id}:update", "UpdateConnector", accept, func(ctx context.Context, e *Env, p port.Principal, in UpdateConnectorRequest) (service.UpdateResult, error) {
		return e.Svc.UpdateConnector(ctx, p, in.ID, in.Version)
	}),
	op("setConnectorLevel", put, "/v1/connectors/{id}/level", "SetConnectorLevel", ok200, func(ctx context.Context, e *Env, p port.Principal, in LevelRequest) (Grants, error) {
		return items[Grants](e.Svc.SetConnectorLevel(ctx, p, in.ID, in.Level))
	}),
	op("removeConnector", del, "/v1/connectors/{id}", "RemoveConnector", gone, func(ctx context.Context, e *Env, p port.Principal, in ConnectorRequest) (none, error) {
		return done(e.Svc.RemoveConnector(ctx, p, in.ID))
	}),
	op("listFunctions", get, "/v1/connectors/{id}/functions", "ListFunctions", ok200, func(ctx context.Context, e *Env, p port.Principal, in ConnectorRequest) (Functions, error) {
		return items[Functions](e.Svc.ListFunctionsOf(ctx, p, in.ID))
	}),
	op("acceptFunction", post, "/v1/connectors/{id}/functions/{name}:accept", "AcceptFunction", ok200, func(ctx context.Context, e *Env, p port.Principal, in AcceptFunctionRequest) (service.AcceptResult, error) {
		return e.Svc.AcceptFunction(ctx, p, in.ID, in.Name, in.Seen, in.KeepOverlay)
	}),
	op("acceptFunctions", post, "/v1/connectors/{id}/functions:accept", "AcceptFunctions", ok200, func(ctx context.Context, e *Env, p port.Principal, in AcceptFunctionsRequest) (AcceptResults, error) {
		return items[AcceptResults](e.Svc.AcceptFunctions(ctx, p, in.ID, in.Seen))
	}),
	op("acceptInstructions", post, "/v1/connectors/{id}/instructions:accept", "AcceptInstructions", ok200, func(ctx context.Context, e *Env, p port.Principal, in ConnectorRequest) (port.Connector, error) {
		return e.Svc.AcceptInstructions(ctx, p, in.ID)
	}),
	op("classifyFunction", put, "/v1/connectors/{id}/functions/{name}/class", "ClassifyFunction", ok200, func(ctx context.Context, e *Env, p port.Principal, in ClassifyFunctionRequest) (port.Function, error) {
		return e.Svc.ClassifyFunction(ctx, p, in.ID, in.Name, in.Class)
	}),
	op("setProjection", put, "/v1/connectors/{id}/functions/{name}/projection", "SetProjection", ok200, func(ctx context.Context, e *Env, p port.Principal, in ProjectionRequest) (port.Function, error) {
		return e.Svc.SetProjection(ctx, p, in.ID, in.Name, in.On)
	}),
	op("disableFunction", post, "/v1/connectors/{id}/functions/{name}:disable", "DisableFunction", ok200, setFunctionDisabled(true)),
	op("enableFunction", post, "/v1/connectors/{id}/functions/{name}:enable", "EnableFunction", ok200, setFunctionDisabled(false)),
	op("listOverlays", get, "/v1/overlays", "ListOverlays", ok200, func(ctx context.Context, e *Env, p port.Principal, in OverlayTargetRequest) (Overlays, error) {
		return items[Overlays](e.Svc.ListOverlays(ctx, p, in.target(), in.Function))
	}),
	op("suggestOverlay", post, "/v1/overlays:suggest", "SuggestOverlay", ok200, func(ctx context.Context, e *Env, p port.Principal, in OverlayTargetRequest) (port.Overlay, error) {
		return e.Svc.SuggestOverlay(ctx, p, in.Connector, in.Function)
	}),
	op("proposeOverlay", post, "/v1/overlays", "ProposeOverlay", http.StatusCreated, func(ctx context.Context, e *Env, p port.Principal, in ProposeOverlayRequest) (port.Overlay, error) {
		return e.Svc.ProposeOverlay(ctx, p, in.target(), in.Function, in.Title, in.Description, in.UpstreamHash)
	}),
	op("acceptOverlay", post, "/v1/overlays/{version}:accept", "AcceptOverlay", ok200, func(ctx context.Context, e *Env, p port.Principal, in OverlayVersionRequest) (port.Overlay, error) {
		return e.Svc.ActivateOverlay(ctx, p, in.target(), in.Function, in.Version, in.UpstreamSeen)
	}),
	op("restoreOverlay", post, "/v1/overlays/{version}:restore", "RestoreOverlay", ok200, func(ctx context.Context, e *Env, p port.Principal, in OverlayVersionRequest) (port.Overlay, error) {
		return e.Svc.ActivateOverlay(ctx, p, in.target(), in.Function, in.Version, in.UpstreamSeen)
	}),
	op("clearOverlay", del, "/v1/overlays", "ClearOverlay", gone, func(ctx context.Context, e *Env, p port.Principal, in OverlayTargetRequest) (none, error) {
		return done(e.Svc.ClearOverlay(ctx, p, in.target(), in.Function))
	}),
	op("putBinding", put, "/v1/connectors/{id}/bindings/{agent}", "PutBinding", ok200, func(ctx context.Context, e *Env, p port.Principal, in PutBindingRequest) (port.Binding, error) {
		return e.Svc.PutBinding(ctx, p, in.binding())
	}),
	op("deleteBinding", del, "/v1/connectors/{id}/bindings/{agent}", "DeleteBinding", gone, func(ctx context.Context, e *Env, p port.Principal, in BindingRequest) (none, error) {
		return done(e.Svc.DeleteBinding(ctx, p, in.ID, in.Agent))
	}),
	op("listBindings", get, "/v1/connectors/{id}/bindings", "ListBindings", ok200, func(ctx context.Context, e *Env, p port.Principal, in ListBindingsRequest) (Bindings, error) {
		return items[Bindings](e.Svc.ListBindings(ctx, p, port.BindingFilter{Connector: in.ID, Agent: in.Agent, Owner: in.Owner}))
	}),
	op("setAgentLevel", put, "/v1/connectors/{id}/bindings/{agent}/level", "SetAgentLevel", ok200, func(ctx context.Context, e *Env, p port.Principal, in AgentLevelRequest) (Grants, error) {
		return items[Grants](e.Svc.SetAgentLevel(ctx, p, in.ID, in.Agent, in.Level))
	}),
	op("createGrant", post, "/v1/grants", "CreateGrant", http.StatusCreated, func(ctx context.Context, e *Env, p port.Principal, in port.Grant) (service.GrantResult, error) {
		return e.Svc.CreateGrant(ctx, p, in)
	}),
	op("deleteGrant", del, "/v1/grants/{id}", "DeleteGrant", gone, func(ctx context.Context, e *Env, p port.Principal, in GrantRequest) (none, error) {
		return done(e.Svc.DeleteGrant(ctx, p, in.ID))
	}),
	op("listGrants", get, "/v1/grants", "ListGrants", ok200, func(ctx context.Context, e *Env, p port.Principal, in ListGrantsRequest) (Grants, error) {
		return items[Grants](e.Svc.ListGrants(ctx, p, port.GrantFilter{ScopeKind: in.ScopeKind, ScopeID: in.ScopeID, Agent: in.Agent, Connector: in.Connector}))
	}),
	op("confirmGrant", post, "/v1/grants/{id}:confirm", "ConfirmGrant", ok200, func(ctx context.Context, e *Env, p port.Principal, in GrantRequest) (port.Grant, error) {
		return e.Svc.ConfirmGrant(ctx, p, in.ID)
	}),
	op("explainAccess", post, "/v1/access:explain", "ExplainAccess", ok200, func(ctx context.Context, e *Env, p port.Principal, in ExplainAccessRequest) (evaluate.Decision, error) {
		return e.Svc.ExplainAccess(ctx, p, in.subject(p), in.Connector, in.Function)
	}),
	op("listAgentFunctions", get, "/v1/agents/{agent}/functions", "ListAgentFunctions", ok200, agentFunctions),
	op("listApprovals", get, "/v1/approvals", "ListApprovals", ok200, func(ctx context.Context, e *Env, p port.Principal, in ListApprovalsRequest) (Approvals, error) {
		f := port.ApprovalFilter{Session: in.Session, Agent: in.Agent, Connector: in.Connector, Since: in.Since}
		if in.State != "" {
			f.States = []port.ApprovalState{in.State}
		}
		return items[Approvals](e.Svc.ListApprovals(ctx, p, f, in.Limit))
	}),
	op("getApproval", get, "/v1/approvals/{id}", "GetApproval", ok200, func(ctx context.Context, e *Env, p port.Principal, in ApprovalRequest) (service.ApprovalDetail, error) {
		return e.Svc.GetApproval(ctx, p, in.ID)
	}),
	op("decideApproval", post, "/v1/approvals/{id}:decide", "DecideApproval", ok200, func(ctx context.Context, e *Env, p port.Principal, in DecideApprovalRequest) (port.Approval, error) {
		return e.Svc.DecideApproval(ctx, p, in.ID, in.Decision, in.Note, in.Channel)
	}, never()),
	op("cancelApproval", post, "/v1/approvals/{id}:cancel", "CancelApproval", ok200, func(ctx context.Context, e *Env, p port.Principal, in ApprovalRequest) (port.Approval, error) {
		return e.Svc.CancelApproval(ctx, p, in.ID)
	}, never()),
	op("listPauses", get, "/v1/pauses", "ListPauses", ok200, func(ctx context.Context, e *Env, p port.Principal, _ none) (Pauses, error) {
		return items[Pauses](e.Svc.ListPauses(ctx, p))
	}),
	op("clearPause", del, "/v1/pauses/{id}", "ClearPause", gone, func(ctx context.Context, e *Env, p port.Principal, in PauseRequest) (none, error) {
		return done(e.Svc.ClearPause(ctx, p, in.ID))
	}, never()),
	watch("watchApprovals", "/v1/approvals:watch", "WatchApprovals", func(ctx context.Context, e *Env, p port.Principal, in WatchApprovalsRequest) ([]port.Event, error) {
		return e.Svc.WatchApprovals(ctx, p, in.After, in.Limit)
	}),
	op("getCall", get, "/v1/calls/{id}", "GetCall", ok200, func(ctx context.Context, e *Env, p port.Principal, in CallRequest) (port.Call, error) {
		return e.Svc.GetCall(ctx, p, in.ID)
	}),
	op("createCredential", post, "/v1/credentials", "CreateCredential", http.StatusCreated, func(ctx context.Context, e *Env, p port.Principal, in service.CredentialInput) (port.Credential, error) {
		return e.Svc.CreateCredential(ctx, p, in)
	}),
	op("beginConnect", post, "/v1/connectors/{id}/credentials:connect", "BeginConnect", ok200, func(ctx context.Context, e *Env, p port.Principal, in BeginConnectRequest) (service.ConnectStart, error) {
		return e.Svc.BeginConnect(ctx, p, in.ID, in.Owner)
	}),
	op("completeConnect", post, "/v1/credentials:complete", "CompleteConnect", ok200, func(ctx context.Context, e *Env, p port.Principal, in CompleteConnectRequest) (port.Credential, error) {
		return e.Svc.CompleteConnect(ctx, p, in.State, in.Code)
	}),
	op("listCredentials", get, "/v1/credentials", "ListCredentials", ok200, func(ctx context.Context, e *Env, p port.Principal, in ListCredentialsRequest) (Credentials, error) {
		return items[Credentials](e.Svc.ListCredentials(ctx, p, port.CredentialFilter{OwnerKind: in.OwnerKind, OwnerID: in.OwnerID, Connector: in.Connector}))
	}),
	op("revokeCredential", del, "/v1/credentials/{id}", "RevokeCredential", gone, func(ctx context.Context, e *Env, p port.Principal, in CredentialRequest) (none, error) {
		return done(e.Svc.RevokeCredential(ctx, p, in.ID))
	}),
	op("putClientRegistration", put, "/v1/issuers/client", "PutClientRegistration", ok200, func(ctx context.Context, e *Env, p port.Principal, in service.ClientRegistrationInput) (port.ClientRegistration, error) {
		return e.Svc.PutClientRegistration(ctx, p, in)
	}),
	op("listCalls", get, "/v1/calls", "ListCalls", ok200, func(ctx context.Context, e *Env, p port.Principal, in ListCallsRequest) (Calls, error) {
		return items[Calls](e.Svc.ListCalls(ctx, p, port.CallFilter{Agent: in.Agent, Session: in.Session, Connector: in.Connector, State: in.State, Since: in.Since}, in.Limit))
	}),
	op("listSources", get, "/v1/sources", "ListSources", ok200, func(ctx context.Context, e *Env, p port.Principal, _ none) (Sources, error) {
		return items[Sources](e.Svc.ListSources(ctx, p))
	}),
	op("rotateKey", post, "/v1/secrets:rotate", "RotateKey", accept, func(ctx context.Context, e *Env, p port.Principal, in RotateKeyRequest) (port.Operation, error) {
		return e.Svc.RotateKey(ctx, p, port.KeyScope{Root: in.Root, Tenant: in.Tenant})
	}),
	op("endSession", post, "/v1/sessions/{id}:end", "EndSession", gone, func(ctx context.Context, e *Env, p port.Principal, in SessionRequest) (none, error) {
		return done(e.Svc.EndSession(ctx, p, in.ID))
	}),
	op("removeAgent", del, "/v1/agents/{id}", "RemoveAgent", gone, func(ctx context.Context, e *Env, p port.Principal, in AgentRequest) (none, error) {
		return done(e.Svc.RemoveAgent(ctx, p, in.ID))
	}),
	op("removePerson", del, "/v1/people/{id}", "RemovePerson", gone, func(ctx context.Context, e *Env, p port.Principal, in PersonRequest) (none, error) {
		return done(e.Svc.RemovePerson(ctx, p, in.ID, in.Workspace))
	}),
	op("restorePerson", post, "/v1/people/{id}:restore", "RestorePerson", gone, func(ctx context.Context, e *Env, p port.Principal, in PersonRequest) (none, error) {
		return done(e.Svc.RestorePerson(ctx, p, in.ID, in.Workspace))
	}),
	op("listRoles", get, "/v1/roles", "ListRoles", ok200, func(ctx context.Context, e *Env, p port.Principal, in ListRolesRequest) (Roles, error) {
		return items[Roles](e.Svc.ListRoles(ctx, p, in.Tenant, port.RoleFilter{Person: in.Person}))
	}),
	op("putRole", put, "/v1/roles", "PutRole", ok200, func(ctx context.Context, e *Env, p port.Principal, in port.RoleAssignment) (port.RoleAssignment, error) {
		return e.Svc.PutRole(ctx, p, in)
	}),
	op("deleteRole", del, "/v1/roles", "DeleteRole", gone, func(ctx context.Context, e *Env, p port.Principal, in port.RoleAssignment) (none, error) {
		return done(e.Svc.DeleteRole(ctx, p, in))
	}),
	op("listGroupMembers", get, "/v1/groups/members", "ListGroupMembers", ok200, func(ctx context.Context, e *Env, p port.Principal, in ListMembersRequest) (GroupMembers, error) {
		return items[GroupMembers](e.Svc.ListGroupMembers(ctx, p, port.MemberFilter{Person: in.Person, Group: in.Group}))
	}),
	op("putGroupMember", put, "/v1/groups/members", "PutGroupMember", ok200, func(ctx context.Context, e *Env, p port.Principal, in port.GroupMember) (port.GroupMember, error) {
		return e.Svc.PutGroupMember(ctx, p, in)
	}),
	op("deleteGroupMember", del, "/v1/groups/members", "DeleteGroupMember", gone, func(ctx context.Context, e *Env, p port.Principal, in port.GroupMember) (none, error) {
		return done(e.Svc.DeleteGroupMember(ctx, p, in))
	}),
	op("getOperation", get, "/v1/operations/{id}", "GetOperation", ok200, func(ctx context.Context, e *Env, p port.Principal, in OperationRequest) (port.Operation, error) {
		return e.Svc.GetOperation(ctx, p, in.ID)
	}),
	op("listEventTypes", get, "/v1/event-types", "ListEventTypes", ok200, eventTypes, public()),
	op("getEventType", get, "/v1/event-types/{type}", "GetEventType", ok200, oneEventType, public()),
	op("listAdapters", get, "/v1/adapters", "ListAdapters", ok200, func(_ context.Context, e *Env, p port.Principal, _ none) (discovery.Document, error) {
		if p.Kind != port.PrincipalSystem {
			return discovery.Document{}, port.Errorf(port.CodePermissionDenied, "only the operator reads the adapters")
		}
		return e.Adapters(), nil
	}),
}
